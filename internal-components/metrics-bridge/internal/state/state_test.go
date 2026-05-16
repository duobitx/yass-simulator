package state

import (
	"testing"
	"time"
)

func TestTrackerRecordAndMatch(t *testing.T) {
	tr := NewTracker(time.Hour, 100)
	now := time.Now()
	tr.RecordPut("abc", "sat-1", 100, now)

	p := tr.MatchReceive("abc")
	if p == nil || p.Source != "sat-1" || p.SizeBytes != 100 {
		t.Fatalf("unexpected match: %#v", p)
	}
	// MatchReceive must not remove the entry — multi-peer delivery.
	if got := tr.MatchReceive("abc"); got == nil {
		t.Fatal("entry was removed after match; expected it to persist")
	}
}

func TestTrackerEvictExpired(t *testing.T) {
	tr := NewTracker(time.Minute, 100)
	now := time.Now()
	tr.RecordPut("old", "sat-1", 1, now.Add(-2*time.Minute))
	tr.RecordPut("fresh", "sat-1", 1, now)

	lost := tr.EvictExpired(now)
	if lost["sat-1"] != 1 {
		t.Fatalf("expected 1 lost from sat-1, got %d", lost["sat-1"])
	}
	if p := tr.MatchReceive("old"); p != nil {
		t.Fatal("old entry should have been evicted")
	}
	if p := tr.MatchReceive("fresh"); p == nil {
		t.Fatal("fresh entry should still be present")
	}
}

func TestTrackerEmptyKey(t *testing.T) {
	tr := NewTracker(time.Hour, 100)
	tr.RecordPut("", "sat-1", 1, time.Now())
	tr.RecordPut("md5", "", 1, time.Now())
	if p := tr.MatchReceive(""); p != nil {
		t.Fatal("empty md5 should not match")
	}
}

func TestIPMapSetReplaces(t *testing.T) {
	m := NewIPMap()
	m.Set("10.0.0.1", "sat-1", "satellite")
	if r := m.Lookup("10.0.0.1"); r.FsNode != "sat-1" || r.NodeType != "satellite" {
		t.Fatalf("unexpected: %#v", r)
	}
	m.Set("10.0.0.2", "sat-1", "satellite")
	if r := m.Lookup("10.0.0.1"); r.FsNode != "" {
		t.Fatalf("stale entry for old IP: %#v", r)
	}
	if r := m.Lookup("10.0.0.2"); r.FsNode != "sat-1" {
		t.Fatalf("missing new ip: %#v", r)
	}
}

func TestTrimPort(t *testing.T) {
	cases := map[string]string{
		"10.0.0.1:8080": "10.0.0.1",
		"10.0.0.1":      "10.0.0.1",
		"":              "",
	}
	for in, want := range cases {
		if got := TrimPort(in); got != want {
			t.Errorf("TrimPort(%q)=%q want %q", in, got, want)
		}
	}
}
