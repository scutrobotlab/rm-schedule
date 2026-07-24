package static

import "testing"

func TestCurrentSeasonZonesIncludes2026RevivalAndFinal(t *testing.T) {
	revival, ok := FindCurrentSeasonZone(617)
	if !ok {
		t.Fatal("expected zone 617 in CurrentSeasonZones")
	}
	if revival.Name != "复活赛" {
		t.Fatalf("revival name = %q, want 复活赛", revival.Name)
	}
	if len(revival.Parts) != 3 {
		t.Fatalf("revival parts = %d, want 3", len(revival.Parts))
	}
	wantRevival := []PartManifest{
		{Index: 0, Name: "A组", Type: "group", Group: "A"},
		{Index: 1, Name: "B组", Type: "group", Group: "B"},
		{Index: 2, Name: "淘汰赛", Type: "knockout", Group: "Knockout"},
	}
	for i, want := range wantRevival {
		got := revival.Parts[i]
		if got != want {
			t.Fatalf("revival part[%d] = %+v, want %+v", i, got, want)
		}
	}

	final, ok := FindCurrentSeasonZone(618)
	if !ok {
		t.Fatal("expected zone 618 in CurrentSeasonZones")
	}
	if final.Name != "全国赛" {
		t.Fatalf("final name = %q, want 全国赛", final.Name)
	}
	if len(final.Parts) != 6 {
		t.Fatalf("final parts = %d, want 6", len(final.Parts))
	}
	wantFinal := []PartManifest{
		{Index: 0, Name: "A组前段", Type: "group", Group: "A"},
		{Index: 1, Name: "B组前段", Type: "group", Group: "B"},
		{Index: 2, Name: "A组后段", Type: "group", Group: "A"},
		{Index: 3, Name: "B组后段", Type: "group", Group: "B"},
		{Index: 4, Name: "淘汰赛败者组", Type: "knockout", Group: "Knockout"},
		{Index: 5, Name: "淘汰赛胜者组", Type: "knockout", Group: "Knockout"},
	}
	for i, want := range wantFinal {
		got := final.Parts[i]
		if got != want {
			t.Fatalf("final part[%d] = %+v, want %+v", i, got, want)
		}
	}
}

func TestArchivedZoneIDsExcludesLive2026Zones(t *testing.T) {
	for _, id := range []string{"617", "618"} {
		if _, ok := ArchivedZoneIDs[id]; ok {
			t.Fatalf("zone %s must not be archived during live season", id)
		}
	}
	for _, id := range []string{"614", "615", "616"} {
		if _, ok := ArchivedZoneIDs[id]; !ok {
			t.Fatalf("regional zone %s should remain archived", id)
		}
	}
}
