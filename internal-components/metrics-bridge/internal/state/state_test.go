package state

import (
	"testing"
	"time"
)

func TestTrackerRecordAndMatch(t *testing.T) {
	tr := NewTracker(time.Hour, 100)
	now := time.Now()
	tr.RecordPut("abc", "file-a", "sat-1", 100, now)

	p, dup := tr.MatchReceive("abc", "file-a", "gs-1")
	if p == nil || dup || p.Source != "sat-1" || p.SizeBytes != 100 {
		t.Fatalf("unexpected match: %#v dup=%v", p, dup)
	}
	// A different receiver is a separate delivery — entry must persist.
	if got, dup := tr.MatchReceive("abc", "file-a", "gs-2"); got == nil || dup {
		t.Fatal("entry was removed after match; expected it to persist for other receivers")
	}
	// The same receiver again is a duplicate receipt — must be flagged.
	if got, dup := tr.MatchReceive("abc", "file-a", "gs-1"); got != nil || !dup {
		t.Fatalf("expected duplicate receipt for gs-1, got %#v dup=%v", got, dup)
	}
}

// EDFS emits RECEIVED events with an empty md5sum; the PUT is joined back via
// the file name index instead.
func TestTrackerNameFallback(t *testing.T) {
	tr := NewTracker(time.Hour, 100)
	now := time.Now()
	tr.RecordPut("md5-x", "sat-1_uc1_0", "sat-1", 42, now)

	// RECEIVED with no md5sum — must still match by name.
	p, dup := tr.MatchReceive("", "sat-1_uc1_0", "gs-1")
	if p == nil || dup || p.Source != "sat-1" || p.SizeBytes != 42 {
		t.Fatalf("name fallback failed: %#v dup=%v", p, dup)
	}
	// Same (file, receiver) again is a duplicate.
	if got, dup := tr.MatchReceive("", "sat-1_uc1_0", "gs-1"); got != nil || !dup {
		t.Fatalf("expected duplicate via name, got %#v dup=%v", got, dup)
	}
	// Unknown name does not match.
	if got, _ := tr.MatchReceive("", "nope", "gs-1"); got != nil {
		t.Fatal("unknown name should not match")
	}
}

func TestTrackerEvictExpired(t *testing.T) {
	tr := NewTracker(time.Minute, 100)
	now := time.Now()
	tr.RecordPut("old", "f-old", "sat-1", 1, now.Add(-2*time.Minute))
	tr.RecordPut("fresh", "f-fresh", "sat-1", 1, now)

	lost := tr.EvictExpired(now)
	if lost["sat-1"] != 1 {
		t.Fatalf("expected 1 lost from sat-1, got %d", lost["sat-1"])
	}
	if p, _ := tr.MatchReceive("old", "f-old", "gs-1"); p != nil {
		t.Fatal("old entry should have been evicted")
	}
	if p, _ := tr.MatchReceive("fresh", "f-fresh", "gs-1"); p == nil {
		t.Fatal("fresh entry should still be present")
	}
}

func TestTrackerEmptyKey(t *testing.T) {
	tr := NewTracker(time.Hour, 100)
	tr.RecordPut("", "", "sat-1", 1, time.Now()) // no md5 and no name -> dropped
	tr.RecordPut("md5", "f", "", 1, time.Now())  // no source -> dropped
	if p, _ := tr.MatchReceive("", "", "gs-1"); p != nil {
		t.Fatal("empty md5 and name should not match")
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
