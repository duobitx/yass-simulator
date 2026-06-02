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
	Name      string
	received  map[string]struct{} // receivers already counted (dedup)
}

type Tracker struct {
	mu          sync.Mutex
	pending     map[string]*PendingPut
	byName      map[string]string // file name -> pending key, for md5-less receives
	deadline    time.Duration
	maxSize     int
	insertOrder []string
}

func NewTracker(deadline time.Duration, maxSize int) *Tracker {
	return &Tracker{
		pending:  make(map[string]*PendingPut),
		byName:   make(map[string]string),
		deadline: deadline,
		maxSize:  maxSize,
	}
}

// RecordPut stores a pending PUT keyed by md5sum, with a secondary index on the
// file name. The name index lets a later RECEIVED that carries no md5sum (the
// EDFS engine emits an empty md5 on receive) still be joined back to its PUT.
// If both md5sum and name are empty the record is dropped — without a join key,
// delivery cannot be reconstructed.
func (t *Tracker) RecordPut(md5sum, name, source string, size int64, when time.Time) {
	if source == "" {
		return
	}
	key := md5sum
	if key == "" {
		key = name
	}
	if key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.pending[key]; exists {
		return
	}
	t.pending[key] = &PendingPut{Source: source, SizeBytes: size, When: when, Name: name}
	if name != "" {
		t.byName[name] = key
	}
	t.insertOrder = append(t.insertOrder, key)
	if len(t.pending) > t.maxSize {
		t.dropOldestLocked()
	}
}

// MatchReceive looks up a pending PUT for the given md5sum (falling back to the
// file name when md5sum is empty or unknown) and records that `receiver` got
// it. It does NOT remove the entry — a single file may be received by multiple
// distinct peers, each a separate delivery. It returns (nil, true) when this
// exact (file, receiver) pair has already been seen (a duplicate receipt —
// engine restart, re-pin, idempotent re-fetch) so the caller can skip
// double-counting, and (nil, false) when neither md5sum nor name resolves to a
// known PUT. The dedup set lives inside the PendingPut, so it is bounded by the
// pending map and freed when the PUT is evicted.
func (t *Tracker) MatchReceive(md5sum, name, receiver string) (*PendingPut, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var p *PendingPut
	if md5sum != "" {
		p = t.pending[md5sum]
	}
	if p == nil && name != "" {
		if key, ok := t.byName[name]; ok {
			p = t.pending[key]
		}
	}
	if p == nil {
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
			if p.Name != "" {
				delete(t.byName, p.Name)
			}
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
	if p, ok := t.pending[k]; ok && p.Name != "" {
		delete(t.byName, p.Name)
	}
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
