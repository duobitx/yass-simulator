package state

import (
	"strings"
	"sync"
	"time"
)

type PendingPut struct {
	Source    string
	SizeBytes int64
	When      time.Time
	received  map[string]struct{} // receivers already counted (dedup)
}

type Tracker struct {
	mu          sync.Mutex
	pending     map[string]*PendingPut
	deadline    time.Duration
	maxSize     int
	insertOrder []string
}

func NewTracker(deadline time.Duration, maxSize int) *Tracker {
	return &Tracker{
		pending:  make(map[string]*PendingPut),
		deadline: deadline,
		maxSize:  maxSize,
	}
}

// RecordPut stores a pending PUT keyed by md5sum. If md5sum is empty the
// record is dropped — without it, delivery cannot be joined back.
func (t *Tracker) RecordPut(md5sum, source string, size int64, when time.Time) {
	if md5sum == "" || source == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.pending[md5sum]; exists {
		return
	}
	t.pending[md5sum] = &PendingPut{Source: source, SizeBytes: size, When: when}
	t.insertOrder = append(t.insertOrder, md5sum)
	if len(t.pending) > t.maxSize {
		t.dropOldestLocked()
	}
}

// MatchReceive looks up a pending PUT for the given md5sum and records that
// `receiver` got it. It does NOT remove the entry — a single file may be
// received by multiple distinct peers, each a separate delivery. It returns
// (nil, true) when this exact (md5sum, receiver) pair has already been seen
// (a duplicate receipt — engine restart, re-pin, idempotent re-fetch) so the
// caller can skip double-counting, and (nil, false) when the md5sum is
// unknown. The dedup set lives inside the PendingPut, so it is bounded by the
// pending map and freed when the PUT is evicted.
func (t *Tracker) MatchReceive(md5sum, receiver string) (*PendingPut, bool) {
	if md5sum == "" {
		return nil, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.pending[md5sum]
	if !ok {
		return nil, false
	}
	if p.received == nil {
		p.received = make(map[string]struct{})
	}
	if _, dup := p.received[receiver]; dup {
		return nil, true
	}
	p.received[receiver] = struct{}{}
	cp := *p
	cp.received = nil
	return &cp, false
}

// EvictExpired returns the (source → target → count) of PUTs older than
// the deadline so the caller can bump yass_file_lost_total.
//
// The "target" is unknown at eviction time (it never arrived) so it is
// returned as empty string; the caller may fold all-empty per-source
// counts into a single "any" target series.
func (t *Tracker) EvictExpired(now time.Time) map[string]int {
	t.mu.Lock()
	defer t.mu.Unlock()
	lost := map[string]int{}
	threshold := now.Add(-t.deadline)
	kept := t.insertOrder[:0]
	for _, k := range t.insertOrder {
		p, ok := t.pending[k]
		if !ok {
			continue
		}
		if p.When.Before(threshold) {
			lost[p.Source]++
			delete(t.pending, k)
			continue
		}
		kept = append(kept, k)
	}
	t.insertOrder = kept
	return lost
}

func (t *Tracker) dropOldestLocked() {
	if len(t.insertOrder) == 0 {
		return
	}
	k := t.insertOrder[0]
	t.insertOrder = t.insertOrder[1:]
	delete(t.pending, k)
}

// IPMap is a tiny thread-safe IP → fsNode + node_type cache fed from
// `online-states/#`.
type IPMap struct {
	mu     sync.RWMutex
	byIP   map[string]NodeRef
	byName map[string]string
}

type NodeRef struct {
	FsNode   string
	NodeType string
}

func NewIPMap() *IPMap { return &IPMap{byIP: map[string]NodeRef{}, byName: map[string]string{}} }

func (m *IPMap) Set(ip, fsNode, nodeType string) {
	if ip == "" || fsNode == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if prev, ok := m.byName[fsNode]; ok && prev != ip {
		delete(m.byIP, prev)
	}
	m.byName[fsNode] = ip
	m.byIP[ip] = NodeRef{FsNode: fsNode, NodeType: nodeType}
}

func (m *IPMap) Lookup(ip string) NodeRef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byIP[ip]
}

// NodeType returns the cached node type for a given fsNode name, or empty
// if not yet seen.
func (m *IPMap) NodeType(fsNode string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ip, ok := m.byName[fsNode]; ok {
		return m.byIP[ip].NodeType
	}
	return ""
}

// TrimPort removes any ":port" suffix; produces a bare IP.
func TrimPort(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[:i]
	}
	return addr
}
