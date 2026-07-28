package handler

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/scutrobotlab/rm-schedule/internal/static"
	"github.com/scutrobotlab/rm-schedule/internal/types"
)

func TestMockMpMatch2025FollowsScore(t *testing.T) {
	var schedule types.ScheduleResp
	if err := json.Unmarshal(static.ScheduleBytes2025, &schedule); err != nil {
		t.Fatal(err)
	}

	tested := 0
	for _, zone := range schedule.Data.Event.Zones.Nodes {
		for _, match := range append(zone.GroupMatches.Nodes, zone.KnockoutMatches.Nodes...) {
			id, err := strconv.Atoi(match.ID)
			if err != nil || match.RedSideWinGameCount == match.BlueSideWinGameCount {
				continue
			}
			mock := mockMpMatch2025(id)
			if mock.RedRate+mock.BlueRate != 1 {
				t.Fatalf("match %d rates do not sum to 1: %+v", id, mock)
			}
			if match.RedSideWinGameCount > match.BlueSideWinGameCount && mock.RedRate <= 0.5 {
				t.Fatalf("match %d red won %d:%d, got %.1f%% support", id, match.RedSideWinGameCount, match.BlueSideWinGameCount, mock.RedRate*100)
			}
			if match.BlueSideWinGameCount > match.RedSideWinGameCount && mock.BlueRate <= 0.5 {
				t.Fatalf("match %d blue won %d:%d, got %.1f%% support", id, match.RedSideWinGameCount, match.BlueSideWinGameCount, mock.BlueRate*100)
			}
			tested++
		}
	}
	if tested == 0 {
		t.Fatal("no decided 2025 matches were tested")
	}
}

func TestMockMpMatch2025UnknownMatch(t *testing.T) {
	mock := mockMpMatch2025(-1)
	if mock.RedRate != -1 || mock.BlueRate != -1 {
		t.Fatalf("unknown match should be unavailable: %+v", mock)
	}
}
