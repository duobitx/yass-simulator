package networking

// engine open ports 3000-3015 (included)
import (
	"fmt"
	"sync"

	_ "github.com/coreos/go-iptables/iptables"
	_ "github.com/florianl/go-tc"
	_ "github.com/mdlayher/netlink"
	"github.com/pkg/errors"
	_ "github.com/pkg/errors"
)

const minPort = 3000
const maxPort = 3015

type trafficDirection byte

const (
	tdIn trafficDirection = iota
	tdOut
)

type NetworkParam struct {
	ToIP         string
	PackageLoss  float32
	PackageDelay float32
}
type networkingHandler struct {
	lock      sync.Mutex
	state     map[string]*NetworkParam
	ipToIndex map[string]int

	//ipt, err := iptables.New()

}

func NewNetworkHandler() *networkingHandler {
	return &networkingHandler{
		lock:      sync.Mutex{},
		state:     make(map[string]*NetworkParam),
		ipToIndex: make(map[string]int),
	}
}

func (h *networkingHandler) Update(param *NetworkParam) error {
	h.lock.Lock()
	defer h.lock.Unlock()
	changed := true
	oldState, ok := h.state[param.ToIP]
	if ok {
		if isAlmostEqual(oldState, param) {
			changed = false
		}
	} else {
		index := len(h.ipToIndex)
		h.ipToIndex[param.ToIP] = index
		err := h.setupMarkRules(param.ToIP)
		if err != nil {
			return errors.Wrapf(err, "cannot setup mark rules for ip=%s", param.ToIP)
		}
	}
	h.state[param.ToIP] = param
	if changed {
		return h.updateNetworkTC(param)
	}
	return nil
}

func (h *networkingHandler) updateNetworkTC(param *NetworkParam) error {
	return nil
}

func (h *networkingHandler) getMarkId(ip string, port int, dir trafficDirection) (int, error) {
	if port < minPort || port > maxPort {
		return -1, fmt.Errorf("invalid port %d", port)
	}
	index, ok := h.ipToIndex[ip]
	if !ok {
		return -1, fmt.Errorf("cannot find markIndex for ip=%s", ip)
	}
	portIndex := port - minPort
	return index*2*(maxPort-minPort) + 2*portIndex + int(dir), nil
}

func (h *networkingHandler) setupMarkRules(ip string) error {
	//err := ipt.Append("mangle", "OUTPUT", "--dport", port, "-j", "MARK", "--set-mark", mark)
	//if err != nil {
	//
	//}
	//
	//// Mark INPUT packets from port
	//err = ipt.Append("mangle", "INPUT",
	//	"-p", "tcp", "--sport", port,
	//	"-j", "MARK", "--set-mark", mark)
	//if err != nil {
	//	log.Fatalf("iptables mark INPUT error: %v", err)
	//}

	return nil
}

func isAlmostEqual(s0 *NetworkParam, s1 *NetworkParam) bool {
	return false // TODO
}
