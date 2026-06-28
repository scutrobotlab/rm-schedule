package bilibili

import "testing"

func TestGetMatchesIncludes2026RegionalZones(t *testing.T) {
	matches := getMatches()
	if matches == nil {
		t.Fatal("getMatches returned nil")
	}

	zones, ok := matches["2026"]
	if !ok {
		t.Fatal("getMatches missing season 2026")
	}

	for _, zoneName := range []string{"南部赛区", "东部赛区", "北部赛区"} {
		if len(zones[zoneName]) == 0 {
			t.Fatalf("season 2026 zone %q has no matches", zoneName)
		}
	}
}
