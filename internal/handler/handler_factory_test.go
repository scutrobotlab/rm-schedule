package handler

import (
	"encoding/json"
	"testing"
)

func TestMergeStaticScheduleZonesKeepsDynamicNewZones(t *testing.T) {
	staticData := []byte(`{"data":{"event":{"zones":{"nodes":[{"id":"614","name":"静态南部"},{"id":"615","name":"静态东部"},{"id":"616","name":"静态北部"}]}}}}`)
	dynamicData := []byte(`{"data":{"event":{"zones":{"nodes":[{"id":"614","name":"动态南部"},{"id":"700","name":"动态全国赛"}]}}}}`)

	merged, err := mergeStaticZones(StaticZoneSeason{
		Data:        staticData,
		ZoneIDs:     map[string]struct{}{"614": {}, "615": {}, "616": {}},
		ZonePath:    []string{"data", "event", "zones", "nodes"},
		ZoneIDField: "id",
	}, dynamicData)
	if err != nil {
		t.Fatalf("mergeStaticZones returned error: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatalf("merged JSON is invalid: %v", err)
	}
	zones, err := zoneNodes(root, []string{"data", "event", "zones", "nodes"})
	if err != nil {
		t.Fatalf("zoneNodes returned error: %v", err)
	}

	wantIDs := []string{"614", "615", "616", "700"}
	wantNames := []string{"静态南部", "静态东部", "静态北部", "动态全国赛"}
	if len(zones) != len(wantIDs) {
		t.Fatalf("got %d zones, want %d", len(zones), len(wantIDs))
	}
	for i, zone := range zones {
		zoneMap := zone.(map[string]any)
		if zoneMap["id"] != wantIDs[i] || zoneMap["name"] != wantNames[i] {
			t.Fatalf("zone %d = %v, want id=%s name=%s", i, zoneMap, wantIDs[i], wantNames[i])
		}
	}
}
