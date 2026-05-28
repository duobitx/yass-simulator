package networking

import (
	"log/slog"

	"github.com/pkg/errors"
)

// ApplyFaultOverlay installs a transient overlay on top of the currently
// programmed orbital rules, used by the hardware-event injector
// (see yass-docs/hardware-events-spec.md §9.1 / §9.2).
//
//   - externalCapBps: if > 0, every per-peer class is reprogrammed so the
//     effective rate becomes min(orbital, externalCapBps). 0 = no cap.
//   - blackHole: if true, every per-peer class is reprogrammed with a
//     1 bps rate and 100% packet loss, effectively dropping all
//     peer-to-peer traffic. The world-controller's own MQTT path to the
//     broker is unaffected because the broker IP is never in h.state.
//
// Idempotent. Calling with `(0, false)` is equivalent to ClearFaultOverlay.
func (h *Handler) ApplyFaultOverlay(externalCapBps int64, blackHole bool) error {
	if h.disabled {
		return nil
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	h.externalCapBps = externalCapBps
	h.blackHole = blackHole
	return h.reapplyAllLocked()
}

// ClearFaultOverlay restores the natural orbital rules. Equivalent to
// ApplyFaultOverlay(0, false).
func (h *Handler) ClearFaultOverlay() error {
	return h.ApplyFaultOverlay(0, false)
}

// reapplyAllLocked re-runs replaceIPProfile / removeIPProfile for every
// peer in h.state so the overlay flags (externalCapBps / blackHole) take
// effect immediately. Caller must hold h.lock.
func (h *Handler) reapplyAllLocked() error {
	for ip, p := range h.state {
		effective := *p
		if h.externalCapBps > 0 && (effective.Bandwidth == 0 || h.externalCapBps < effective.Bandwidth) {
			effective.Bandwidth = h.externalCapBps
		}
		if h.blackHole {
			effective.Bandwidth = 1
			effective.PackageLoss = 100
		}
		if effective.isFullyBlocking() {
			if err := h.removeIPProfile(ip); err != nil {
				return errors.Wrapf(err, "overlay remove %s", ip)
			}
			continue
		}
		if err := h.replaceIPProfile(&effective); err != nil {
			return errors.Wrapf(err, "overlay replace %s", ip)
		}
	}
	slog.Default().Info("Fault overlay re-applied", "externalCapBps", h.externalCapBps, "blackHole", h.blackHole, "peers", len(h.state))
	return nil
}
