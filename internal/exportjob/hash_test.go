package exportjob

import (
	"testing"

	"github.com/scutrobotlab/rm-schedule/internal/static"
)

func TestZoneHashFromSchedule(t *testing.T) {
	hash614, err := zoneHashFromSchedule(static.ScheduleBytes2026, 614)
	if err != nil {
		t.Fatalf("zone 614 hash: %v", err)
	}
	if hash614 == "" || hash614[:7] != "sha256:" {
		t.Fatalf("unexpected hash format: %q", hash614)
	}

	hash614Again, err := zoneHashFromSchedule(static.ScheduleBytes2026, 614)
	if err != nil {
		t.Fatalf("zone 614 hash again: %v", err)
	}
	if hash614 != hash614Again {
		t.Fatalf("hash not stable: %q vs %q", hash614, hash614Again)
	}

	hash616, err := zoneHashFromSchedule(static.ScheduleBytes2026, 616)
	if err != nil {
		t.Fatalf("zone 616 hash: %v", err)
	}
	if hash616 == hash614 {
		t.Fatalf("different zones should not share hash")
	}
}

func TestExtractZoneNodeMissing(t *testing.T) {
	node, err := extractZoneNode(static.ScheduleBytes2026, 99999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node != nil {
		t.Fatalf("expected nil node for missing zone")
	}
}
