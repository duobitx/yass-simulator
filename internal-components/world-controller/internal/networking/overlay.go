package networking

import (
	"log/slog"
	"net"

	"github.com/pkg/errors"
)

// ApplyFaultOverlay installs a transient overlay on top of the currently
// programmed orbital rules, used by the hardware-event injector
// (see yass-docs/hardware-events-spec.md §9.1 / §9.2).
//
//   - externalCapBps: if > 0, every per-peer class is reprogrammed so the
//     effective rate becomes min(orbital, externalCapBps). 0 = no cap.
//   - reductionPct: if > 0, every per-peer rate is reduced to
//     (100-reductionPct)% of its orbital value before any cap is applied.
//   - blackHole: if true, every peer is fully blocked — the per-peer egress
//     class is removed (so egress falls through to the drop class) and an
//     ingress drop filter is installed, dropping all peer-to-peer traffic in
//     both directions. The world-controller's own MQTT path to the broker is
//     unaffected because the broker IP is never in h.state.
//
// Idempotent. Calling with `(0, false)` is equivalent to ClearFaultOverlay.
func (h *Handler) ApplyFaultOverlay(externalCapBps int64, reductionPct int32, blackHole bool) error {
	if h.disabled {
		return nil
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	h.externalCapBps = externalCapBps
	h.reductionPct = reductionPct
	h.blackHole = blackHole
	return h.reapplyAllLocked()
}

// ClearFaultOverlay restores the natural orbital rules. Equivalent to
// ApplyFaultOverlay(0, 0, false).
func (h *Handler) ClearFaultOverlay() error {
	return h.ApplyFaultOverlay(0, 0, false)
}

// applyOverlayLocked mutates `p` to reflect the active fault overlay on top of
// its orbital values: first a multiplicative bandwidth reduction
// (reductionPct), then an absolute cap (externalCapBps, applied as a min), then
// a full black-hole. Caller must hold h.lock.
func (h *Handler) applyOverlayLocked(p *NetworkParam) {
	if h.reductionPct > 0 && p.Bandwidth > 0 {
		p.Bandwidth = p.Bandwidth * int64(100-h.reductionPct) / 100
		if p.Bandwidth < 1 {
			p.Bandwidth = 1 // a reduction throttles; it must not fully block
		}
	}
	if h.externalCapBps > 0 && (p.Bandwidth == 0 || h.externalCapBps < p.Bandwidth) {
		p.Bandwidth = h.externalCapBps
	}
	if h.blackHole {
		p.Bandwidth = 1
		p.PackageLoss = 100
	}
}

// reapplyAllLocked re-runs replaceIPProfile / removeIPProfile for every
// peer in h.state so the overlay flags take effect immediately. Caller must
// hold h.lock.
func (h *Handler) reapplyAllLocked() error {
	for ip, p := range h.state {
		effective := *p
		h.applyOverlayLocked(&effective)
		if effective.isFullyBlocking() {
			if err := h.removeIPProfile(ip); err != nil {
				return errors.Wrapf(err, "overlay remove %s", ip)
			}
			// Egress drops via the default class; under a NetworkFailure also
			// drop ingress (no default ingress drop exists).
			if h.blackHole {
				if dst := net.ParseIP(ip); dst != nil {
					if err := h.addIngressFilters(dst, true); err != nil {
						return errors.Wrapf(err, "overlay ingress drop %s", ip)
					}
				}
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
