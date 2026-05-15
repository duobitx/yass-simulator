package networking

// Engine open ports: 4000-5000 and 9000-9999 — covers IPFS swarm (4001), tus (9090), ipfs-cluster (9094, 9096).
// Excludes 8080 (control-plane: experiment-executor / events-webapp / web-ui).
import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sync"

	proto "github.com/duobitx/yass-simulator/internal-components/go-common/proto/go"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
)

// FsNode data-plane port ranges (IPFS swarm 4001, ipfs-cluster 9094/9096, tus 9090, UDP 9098-9100).
// Excludes control-plane 8080 used by experiment-executor / events-webapp / web-ui.
var managedPortRanges = [...]struct {
	from, to uint16
}{
	{4000, 5000},
	{9000, 9999},
}

func NetworkParamFromFsNodeUpdateNetworkParamEntry(in *proto.FsNodeUpdateNetworkParamEntry) NetworkParam {
	return NetworkParam{
		ToIP:         in.Ip,
		PackageLoss:  in.PackageLoss * 100, // proto value is 0..1, netem expects 0..100
		PackageDelay: in.PackageDelay,
		Bandwidth:    int64(in.Bandwidth),
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

type Handler struct {
	lock       sync.Mutex
	state      map[string]*NetworkParam
	netmask    net.IPMask
	defEthLink netlink.Link
	disabled   bool
}

func NewNetworkHandler(disabled bool) (*Handler, error) {
	if disabled {
		slog.Default().Info("Networking manipulation disabled; skipping netlink setup")
		return &Handler{
			state:    make(map[string]*NetworkParam),
			disabled: true,
		}, nil
	}
	netMask, defaultNetworkInterfaceName, err := findDefaultNetworkNetmask()
	if err != nil {
		return nil, err
	}
	slog.Default().Info("Default network interface", "interface", defaultNetworkInterfaceName)
	defEthLink, err := netlink.LinkByName(defaultNetworkInterfaceName)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get netlink for %s", defaultNetworkInterfaceName)
	}
	h := &Handler{
		lock:       sync.Mutex{},
		state:      make(map[string]*NetworkParam),
		netmask:    netMask,
		defEthLink: defEthLink,
	}
	// Setup ingress qdisc for incoming traffic stats
	if err := h.setupIngressQdisc(); err != nil {
		return nil, errors.Wrap(err, "failed to setup ingress qdisc")
	}
	return h, nil
}

func (h *Handler) Update(networkParams []NetworkParam) error {
	if h.disabled {
		return nil
	}
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
	for k := range h.state {
		currentProfiles[k] = false
	}
	for _, param := range networkParams {
		currentProfiles[param.ToIP] = true
		if oldState, ok := h.state[param.ToIP]; ok && isAlmostEqual(oldState, &param) {
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
				delete(h.state, ip)
				slog.Default().Info("ipProfile removed as the IP is not visible anymore", "ip", ip)
			}
		}
	}

	return jeh.AsError()
}

func (h *Handler) replaceIPProfile(param *NetworkParam) error {
	cid, err := h.getCID(param.ToIP)
	if err != nil {
		return errors.Wrapf(err, "error generating CID for ip=%s", param.ToIP)
	}
	slog.Default().Info("Adding ipProfile", "ip", param.ToIP, "cid", cid, "param", param)
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
		for _, r := range managedPortRanges {
			if err := netlink.FilterReplace(mkFlower(p, r.from, r.to)); err != nil {
				return errors.Wrap(err, "egress FilterReplace (tcp/udp)")
			}
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

	// Add ingress filters for incoming traffic stats
	if err := h.addIngressFilters(dst); err != nil {
		return errors.Wrap(err, "failed to add ingress filters")
	}

	return nil
}

func (h *Handler) addIngressFilters(srcIP net.IP) error {
	protoTCP := nl.IPProto(unix.IPPROTO_TCP)
	protoUDP := nl.IPProto(unix.IPPROTO_UDP)
	protoICMP := nl.IPProto(unix.IPPROTO_ICMP)

	fa := netlink.FilterAttrs{
		LinkIndex: h.defEthLink.Attrs().Index,
		Parent:    netlink.HANDLE_INGRESS,
		Priority:  5,
		Protocol:  uint16(unix.ETH_P_IP),
	}

	// Helper to create ingress flower filter
	mkIngressFlower := func(p *nl.IPProto, minP, maxP uint16) *netlink.Flower {
		fl := &netlink.Flower{
			FilterAttrs:     fa,
			EthType:         unix.ETH_P_IP,
			SrcIP:           srcIP,
			SrcPortRangeMin: minP,
			SrcPortRangeMax: maxP,
			IPProto:         p,
		}
		// Action to count packets (PIPE means continue processing)
		fl.Actions = []netlink.Action{
			&netlink.GenericAction{
				ActionAttrs: netlink.ActionAttrs{
					Action: netlink.TC_ACT_PIPE,
				},
			},
		}
		return fl
	}

	// Add TCP and UDP filters for each managed port range
	for _, p := range []*nl.IPProto{&protoTCP, &protoUDP} {
		for _, r := range managedPortRanges {
			if err := netlink.FilterReplace(mkIngressFlower(p, r.from, r.to)); err != nil {
				return errors.Wrap(err, "ingress FilterReplace (tcp/udp)")
			}
		}
	}

	// Add ICMP filter
	flICMP := &netlink.Flower{
		FilterAttrs: fa,
		EthType:     unix.ETH_P_IP,
		SrcIP:       srcIP,
		IPProto:     &protoICMP,
	}
	flICMP.Actions = []netlink.Action{
		&netlink.GenericAction{
			ActionAttrs: netlink.ActionAttrs{
				Action: netlink.TC_ACT_PIPE,
			},
		},
	}
	if err := netlink.FilterReplace(flICMP); err != nil {
		return errors.Wrap(err, "ingress FilterReplace (icmp)")
	}

	return nil
}

// removeIPProfile assumes h.lock is already held by the caller.
func (h *Handler) removeIPProfile(ip string) error {
	cid, err := h.getCID(ip)
	if err != nil {
		return errors.Wrapf(err, "error generating CID for ip=%s", ip)
	}

	classId := netlink.MakeHandle(1, cid)
	slog.Default().Info("Removing ipProfile", "ip", ip, "cid", cid, "classId", classId)

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

	// Delete ingress filters for this IP
	if err := h.removeIngressFilters(ip); err != nil {
		slog.Default().Warn("Failed to remove ingress filters", "ip", ip, "error", err)
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

func (h *Handler) removeIngressFilters(ip string) error {
	srcIP := net.ParseIP(ip)
	if srcIP == nil {
		return fmt.Errorf("invalid IP: %s", ip)
	}

	// Get all ingress filters
	filters, err := netlink.FilterList(h.defEthLink, netlink.HANDLE_INGRESS)
	if err != nil {
		return errors.Wrap(err, "failed to list ingress filters")
	}

	// Delete filters matching this source IP
	for _, f := range filters {
		if fl, ok := f.(*netlink.Flower); ok {
			if fl.SrcIP != nil && fl.SrcIP.Equal(srcIP) {
				if err := netlink.FilterDel(fl); err != nil {
					slog.Default().Warn("Failed to delete ingress filter", "ip", ip, "error", err)
				}
			}
		}
	}

	return nil
}

func (h *Handler) getCID(ip string) (uint16, error) {
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

func (h *Handler) GetTrafficStats() ([]*proto.TrafficStats, error) {
	if h.disabled {
		return nil, nil
	}
	h.lock.Lock()
	defer h.lock.Unlock()

	stats := make([]*proto.TrafficStats, 0, len(h.state))

	for ip := range h.state {
		if h.state[ip] == nil {
			continue
		}
		cid, err := h.getCID(ip)
		if err != nil {
			slog.Default().Warn("Failed to get CID for IP", "ip", ip, "error", err)
			continue
		}

		classId := netlink.MakeHandle(1, cid)

		// Get egress (outgoing) stats from HTB class
		classes, err := netlink.ClassList(h.defEthLink, netlink.MakeHandle(1, 0))
		if err != nil {
			slog.Default().Warn("Failed to list classes", "error", err)
			continue
		}

		var bytesOut, packetsOut uint64
		for _, class := range classes {
			if class.Attrs().Handle == classId {
				attrs := class.Attrs()
				if attrs.Statistics != nil && attrs.Statistics.Basic != nil {
					bytesOut = attrs.Statistics.Basic.Bytes
					packetsOut = uint64(attrs.Statistics.Basic.Packets)
				}
				break
			}
		}

		// Get ingress (incoming) stats
		bytesIn, packetsIn := h.getIngressStats(ip)

		stats = append(stats, &proto.TrafficStats{
			Ip:                   ip,
			TotalBytesSent:       bytesOut,
			TotalBytesReceived:   bytesIn,
			TotalPacketsSent:     packetsOut,
			TotalPacketsReceived: packetsIn,
		})
	}

	return stats, nil
}

func (h *Handler) getIngressStats(ip string) (uint64, uint64) {
	// Get ingress filter statistics for this IP
	filters, err := netlink.FilterList(h.defEthLink, netlink.HANDLE_INGRESS)
	if err != nil {
		return 0, 0
	}

	srcIP := net.ParseIP(ip)
	if srcIP == nil {
		return 0, 0
	}

	var totalBytes, totalPackets uint64
	for _, filter := range filters {
		if fl, ok := filter.(*netlink.Flower); ok {
			if fl.SrcIP != nil && fl.SrcIP.Equal(srcIP) {
				// Get statistics from filter actions if available
				if len(fl.Actions) > 0 {
					for _, action := range fl.Actions {
						attrs := action.Attrs()
						if attrs != nil && attrs.Statistics != nil && attrs.Statistics.Basic != nil {
							totalBytes += attrs.Statistics.Basic.Bytes
							totalPackets += uint64(attrs.Statistics.Basic.Packets)
						}
					}
				}
			}
		}
	}

	return totalBytes, totalPackets
}

func (h *Handler) setupIngressQdisc() error {
	// Create ingress qdisc if not exists
	ingress := &netlink.Ingress{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: h.defEthLink.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_INGRESS,
		},
	}

	if err := netlink.QdiscAdd(ingress); err != nil {
		// Check if already exists
		if err.Error() == "file exists" {
			slog.Default().Debug("Ingress qdisc already exists")
			return nil
		}
		return err
	}

	slog.Default().Info("Ingress qdisc created successfully")
	return nil
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
		// Prefer the interface that owns the IPv4 default route — same selection that traffic.sh uses.
		routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
		if err == nil {
			for _, r := range routes {
				if r.Dst != nil {
					continue
				}
				link, err := netlink.LinkByIndex(r.LinkIndex)
				if err != nil {
					continue
				}
				iface, err := net.InterfaceByName(link.Attrs().Name)
				if err != nil {
					continue
				}
				defIface = iface
				break
			}
		}
		if defIface == nil {
			netIfaces := goutils.Filter(ifacesAll, func(element net.Interface) bool { return element.Name != "lo" && element.Name != "loopback" })
			if len(netIfaces) == 0 {
				return nil, "", fmt.Errorf("no non-loopback interfaces found")
			}
			if len(netIfaces) > 1 {
				slog.Default().Warn("no default route found and multiple non-loopback interfaces — falling back to first", "interfaces", netIfaces, "picked", netIfaces[0].Name)
			}
			defIface = &netIfaces[0]
		}
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
