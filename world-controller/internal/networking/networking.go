package networking

// engine open ports 3000-3015 (included)
import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sync"

	"github.com/duobitx/yass-internal-components/go-common/proto"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
)

const (
	portsFrom = 3000
	portsTo   = 3020
)

func NetworkParamFromFsNodeUpdateNetworkParamEntry(in *proto.FsNodeUpdateNetworkParamEntry) NetworkParam {
	return NetworkParam{
		ToIP:         in.Ip,
		PackageLoss:  in.PackageLoss * 100,
		PackageDelay: in.PackageDelay,
		Bandwidth:    10 * 1024 * 1024, // As for now 10mbits, TODO limit bandwidth when we know know - bandwith=f(distance)
	}
}

type NetworkParam struct {
	ToIP         string
	PackageLoss  float32 // percent, e.g. 2.5 means 2.5%
	PackageDelay float32 // milliseconds
	Bandwidth    int64   // bits per second
}

func (np NetworkParam) isFullyBlocking() bool {
	return np.PackageLoss >= 100.0 || np.Bandwidth <= 0
}

type NetworkingHandler struct {
	lock       sync.Mutex
	state      map[string]*NetworkParam
	netmask    net.IPMask
	defEthLink netlink.Link
}

func NewNetworkHandler() (*NetworkingHandler, error) {
	netMask, defaultNetworkInterfaceName, err := findDefaultNetworkNetmask()
	if err != nil {
		return nil, err
	}
	slog.Default().Info("Default network interface", "interface", defaultNetworkInterfaceName)
	defEthLink, err := netlink.LinkByName(defaultNetworkInterfaceName)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get netlink for %s", defaultNetworkInterfaceName)
	}
	return &NetworkingHandler{
		lock:       sync.Mutex{},
		state:      make(map[string]*NetworkParam),
		netmask:    netMask,
		defEthLink: defEthLink,
	}, nil
}

func (h *NetworkingHandler) Update(networkParams []NetworkParam) error {
	jeh := goutils.JoinErrorHelper{}
	stopWatch := goutils.NewStopWatch()
	stopWatch.Start()
	defer func() {
		if fErr := jeh.AsError(); fErr != nil {
			errorCount := 1
			type multi interface{ Unwrap() []error }
			if m, ok := fErr.(multi); ok {
				errorCount = len(m.Unwrap())
			}
			slog.Default().Error("Network parameters updated with errors", "opDuration", stopWatch.GetDuration(), "networkParamsCount", len(networkParams), "errorsCount", errorCount)
		} else {
			slog.Default().Info("Network parameters updated successfully", "opDuration", stopWatch.GetDuration(), "networkParamsCount", len(networkParams))
		}
	}()
	h.lock.Lock()
	defer h.lock.Unlock()
	currentProfiles := make(map[string]bool)
	for k, v := range h.state {
		if v != nil {
			currentProfiles[k] = false
		}
	}
	for _, param := range networkParams {
		currentProfiles[param.ToIP] = true
		oldState, ok := h.state[param.ToIP]
		if ok && oldState != nil && isAlmostEqual(oldState, &param) {
			continue
		}
		h.state[param.ToIP] = &param
		if param.isFullyBlocking() {
			if err := h.removeIPProfile(param.ToIP); err != nil {
				jeh.Append(errors.Wrapf(err, "error removing ipProfile for %s", param.ToIP))
			} else {
				slog.Default().Debug("ipProfile removed", "ip", param.ToIP)
			}
		} else {
			if err := h.replaceIPProfile(&param); err != nil {
				jeh.Append(errors.Wrapf(err, "error applying ipProfile for %s", param.ToIP))
			} else {
				slog.Default().Debug("ipProfile applied", "ip", param.ToIP, "param", param)
			}
		}
	}
	// block traffic for all IPs that are not visible anymore
	for ip, v := range currentProfiles {
		if !v { // ip not visible anymore
			if err := h.removeIPProfile(ip); err != nil {
				jeh.Append(errors.Wrapf(err, "error removing ipProfile for %s as it's not visible anymore", ip))
			} else {
				h.state[ip] = nil
				slog.Default().Info("ipProfile removed as the IP is not visible anymore", "ip", ip)
			}
		}
	}

	return jeh.AsError()
}

func (h *NetworkingHandler) replaceIPProfile(param *NetworkParam) error {
	cid, err := h.getCID(param.ToIP)
	if err != nil {
		return errors.Wrapf(err, "error generating CID for ip=%s", param.ToIP)
	}

	// Validate destination IP
	dst := net.ParseIP(param.ToIP)
	if dst == nil {
		return fmt.Errorf("invalid dst ip: %s", param.ToIP)
	}

	// Egress: create/update HTB class 1:cid and attach netem
	classIdEgress := netlink.MakeHandle(1, cid)
	parentEgress := netlink.MakeHandle(1, 0) // root 1:

	htbClass := netlink.NewHtbClass(
		netlink.ClassAttrs{
			LinkIndex: h.defEthLink.Attrs().Index,
			Parent:    parentEgress,
			Handle:    classIdEgress,
		},
		netlink.HtbClassAttrs{
			Rate: uint64(param.Bandwidth),
			Ceil: uint64(param.Bandwidth),
		},
	)
	if err := netlink.ClassReplace(htbClass); err != nil {
		return errors.Wrap(err, "egress ClassReplace")
	}

	// Netem under the class for delay/loss
	netemE := netlink.NewNetem(
		netlink.QdiscAttrs{
			LinkIndex: h.defEthLink.Attrs().Index,
			Parent:    classIdEgress,
			Handle:    netlink.MakeHandle(cid, 0),
		},
		netlink.NetemQdiscAttrs{
			Latency: uint32(param.PackageDelay * 1000), // ms -> us
			Loss:    param.PackageLoss,
		},
	)
	if err := netlink.QdiscReplace(netemE); err != nil {
		return errors.Wrapf(err, "qdisc replace for %v", netemE)
	}
	// Clean previous egress filters for this class
	if filters, err := netlink.FilterList(h.defEthLink, netlink.MakeHandle(1, 0)); err == nil {
		for _, f := range filters {
			if fl, ok := f.(*netlink.Flower); ok {
				if fl.ClassId == classIdEgress {
					_ = netlink.FilterDel(fl)
				}
			}
		}
	}
	// Add TCP/UDP flower filters for port range and ICMP for the destination IP
	protoTCP := nl.IPProto(unix.IPPROTO_TCP)
	protoUDP := nl.IPProto(unix.IPPROTO_UDP)
	protoICMP := nl.IPProto(unix.IPPROTO_ICMP)
	fa := netlink.FilterAttrs{
		LinkIndex: h.defEthLink.Attrs().Index,
		Parent:    netlink.MakeHandle(1, 0),
		Priority:  5,
		Protocol:  uint16(unix.ETH_P_IP),
	}
	// Helper to create flower filter
	mkFlower := func(p *nl.IPProto, minP, maxP uint16) *netlink.Flower {
		return &netlink.Flower{
			FilterAttrs:     fa,
			ClassId:         classIdEgress,
			EthType:         unix.ETH_P_IP,
			DestIP:          dst,
			DstPortRangeMin: minP,
			DstPortRangeMax: maxP,
			IPProto:         p,
		}
	}

	for _, p := range []*nl.IPProto{&protoTCP, &protoUDP} {
		if err := netlink.FilterReplace(mkFlower(p, portsFrom, portsTo)); err != nil {
			return errors.Wrap(err, "egress FilterReplace (tcp/udp)")
		}
	}
	// ICMP
	flICMP := &netlink.Flower{
		FilterAttrs: fa,
		ClassId:     classIdEgress,
		EthType:     unix.ETH_P_IP,
		DestIP:      dst,
		IPProto:     &protoICMP,
	}
	if err := netlink.FilterReplace(flICMP); err != nil {
		return errors.Wrap(err, "egress FilterReplace (icmp)")
	}
	return nil
}

func (h *NetworkingHandler) removeIPProfile(ip string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	cid, err := h.getCID(ip)
	if err != nil {
		return errors.Wrapf(err, "error generating CID for ip=%s", ip)
	}

	classId := netlink.MakeHandle(1, cid)

	// Delete matching flower filters (parent 1:) that steer to this class
	if filters, err := netlink.FilterList(h.defEthLink, netlink.MakeHandle(1, 0)); err == nil {
		for _, f := range filters {
			if fl, ok := f.(*netlink.Flower); ok {
				if fl.ClassId == classId {
					_ = netlink.FilterDel(fl)
				}
			}
		}
	}

	// Delete netem qdisc attached under the class (handle cid:0)
	if qdiscs, err := netlink.QdiscList(h.defEthLink); err == nil {
		for _, q := range qdiscs {
			attrs := q.Attrs()
			// Match by parent == classId or by handle's major == cid
			if attrs.Parent == classId || (attrs.Handle>>16) == uint32(cid) {
				_ = netlink.QdiscDel(q)
			}
		}
	}

	// Delete the HTB class 1:cid
	htbClass := netlink.NewHtbClass(
		netlink.ClassAttrs{
			LinkIndex: h.defEthLink.Attrs().Index,
			Parent:    netlink.MakeHandle(1, 0),
			Handle:    classId,
		},
		netlink.HtbClassAttrs{},
	)
	_ = netlink.ClassDel(htbClass)
	return nil
}

func (h *NetworkingHandler) getCID(ip string) (uint16, error) {
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return 0, fmt.Errorf("invalid ipAddr '%s'", ip)
	}
	ip4 := ipAddr.To4()
	if ip4 == nil {
		return 0, fmt.Errorf("cannot convert ip ti ipv4 '%s'", ip)
	}
	ipUint := binary.BigEndian.Uint32(ip4)
	maskUint := binary.BigEndian.Uint32(h.netmask)
	result := ipUint &^ maskUint
	return uint16(result & 0xFFFF), nil
}

func isAlmostEqual(s0 *NetworkParam, s1 *NetworkParam) bool {
	if math.Abs(float64(s0.Bandwidth-s1.Bandwidth)) > 1024 {
		return false
	}
	if math.Abs(float64(s0.PackageDelay-s1.PackageDelay)) > 10 {
		return false
	}
	if math.Abs(float64(s0.PackageLoss-s1.PackageLoss)) > 1 {
		return false
	}
	if s0.isFullyBlocking() != s1.isFullyBlocking() {
		return false
	}
	return true
}

func findDefaultNetworkNetmask() (net.IPMask, string, error) {
	ifacesAll, err := net.Interfaces()
	if err != nil {
		return nil, "", err
	}
	var defIface *net.Interface
	ifaceEnv := goutils.Env("IFACE", "")
	if ifaceEnv != "" {
		iface, err := net.InterfaceByName(ifaceEnv)
		if err != nil {
			return nil, "", errors.Wrapf(err, "cannot get network interface by name '%s'", ifaceEnv)
		}
		defIface = iface
	} else {
		slog.Default().Info("Try to detect default network interface")
		netIfaces := goutils.Filter(ifacesAll, func(element net.Interface) bool { return element.Name != "lo" && element.Name != "loopback" })
		if len(netIfaces) == 0 {
			return nil, "", fmt.Errorf("no non-loopback interfaces found")
		}
		if len(netIfaces) > 1 {
			return nil, "", fmt.Errorf("more then one network interfaces: %+v", netIfaces)
		}
		defIface = &netIfaces[0]
	}
	addrs, err := defIface.Addrs()
	if err != nil {
		return nil, "", errors.Wrapf(err, "cannot get addresses for network interface by name '%s'", ifaceEnv)
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
			return ipNet.Mask, defIface.Name, nil
		}
	}
	return nil, "", fmt.Errorf("no IPv4 address on interface %s", defIface.Name)
}
