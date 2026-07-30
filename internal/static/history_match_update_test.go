package static

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"
)

const historyMatchScheduleEnv = "HISTORY_MATCH_SCHEDULE_JSON"

type historySchedule struct {
	Data struct {
		Event struct {
			Zones struct {
				Nodes []historyZone `json:"nodes"`
			} `json:"zones"`
		} `json:"event"`
	} `json:"data"`
}

type historyZone struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	ZoneType        string         `json:"zoneType"`
	Groups          historyGroups  `json:"groups"`
	GroupMatches    historyMatches `json:"groupMatches"`
	KnockoutMatches historyMatches `json:"knockoutMatches"`
}

type historyGroups struct {
	Nodes []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"nodes"`
}

type historyMatches struct {
	Nodes []historyScheduleMatch `json:"nodes"`
}

type historyScheduleMatch struct {
	GroupID              *string `json:"groupId"`
	OrderNumber          int     `json:"orderNumber"`
	Slug                 *string `json:"slug"`
	SlugName             string  `json:"slugName"`
	Status               string  `json:"status"`
	RedSideWinGameCount  int     `json:"redSideWinGameCount"`
	BlueSideWinGameCount int     `json:"blueSideWinGameCount"`
	RedSide              historySide
	BlueSide             historySide
}

type historySide struct {
	Player *struct {
		Team struct {
			Name        string `json:"name"`
			CollegeName string `json:"collegeName"`
		} `json:"team"`
	} `json:"player"`
}

// TestUpdateHistoryMatchFrom2026Schedule is also the repeatable update command.
// To use a freshly downloaded file:
//
//	HISTORY_MATCH_SCHEDULE_JSON=/path/to/schedule.json go test ./internal/static \
//	  -run TestUpdateHistoryMatchFrom2026Schedule -count=1
func TestUpdateHistoryMatchFrom2026Schedule(t *testing.T) {
	schedulePath := os.Getenv(historyMatchScheduleEnv)
	if schedulePath == "" {
		schedulePath = "./season_2026/schedule.json"
	}

	count, err := updateHistoryMatchFromSchedule(schedulePath, "./history_match.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if count != 266 {
		t.Fatalf("2026 regional match count = %d, want 266", count)
	}
	if err := convertAndSaveToJSON(
		"./history_match.tsv",
		"./history_match.json",
		[]string{"group", "redTeamName", "blueTeamName"},
	); err != nil {
		t.Fatal(err)
	}
}

func updateHistoryMatchFromSchedule(schedulePath, historyPath string) (int, error) {
	scheduleBytes, err := os.ReadFile(schedulePath)
	if err != nil {
		return 0, fmt.Errorf("read schedule: %w", err)
	}
	var schedule historySchedule
	if err := json.Unmarshal(scheduleBytes, &schedule); err != nil {
		return 0, fmt.Errorf("parse schedule: %w", err)
	}

	file, err := os.Open(historyPath)
	if err != nil {
		return 0, fmt.Errorf("open history TSV: %w", err)
	}
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	rows, err := reader.ReadAll()
	closeErr := file.Close()
	if err != nil {
		return 0, fmt.Errorf("read history TSV: %w", err)
	}
	if closeErr != nil {
		return 0, fmt.Errorf("close history TSV: %w", closeErr)
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("history TSV is empty")
	}

	result := [][]string{rows[0]}
	maxOrder := 0
	for _, row := range rows[1:] {
		if len(row) != len(rows[0]) {
			return 0, fmt.Errorf("history TSV row has %d columns, want %d", len(row), len(rows[0]))
		}
		if row[1] == "2026" {
			continue
		}
		order, err := strconv.Atoi(row[0])
		if err != nil {
			return 0, fmt.Errorf("parse history order %q: %w", row[0], err)
		}
		if order > maxOrder {
			maxOrder = order
		}
		result = append(result, row)
	}

	sort.Slice(schedule.Data.Event.Zones.Nodes, func(i, j int) bool {
		left, _ := strconv.Atoi(schedule.Data.Event.Zones.Nodes[i].ID)
		right, _ := strconv.Atoi(schedule.Data.Event.Zones.Nodes[j].ID)
		return left < right
	})

	added := 0
	for _, zone := range schedule.Data.Event.Zones.Nodes {
		if zone.ZoneType != "GROUP_ZONE" {
			continue
		}
		groupNames := make(map[string]string, len(zone.Groups.Nodes))
		for _, group := range zone.Groups.Nodes {
			groupNames[group.ID] = group.Name
		}
		matches := append([]historyScheduleMatch(nil), zone.GroupMatches.Nodes...)
		matches = append(matches, zone.KnockoutMatches.Nodes...)
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].OrderNumber < matches[j].OrderNumber
		})
		for _, match := range matches {
			if match.Status != "DONE" || match.RedSide.Player == nil || match.BlueSide.Player == nil {
				continue
			}
			group := ""
			if match.GroupID != nil {
				group = groupNames[*match.GroupID] + "组第" + match.SlugName + "轮"
			} else if match.Slug != nil {
				group = *match.Slug
			}
			maxOrder++
			added++
			result = append(result, []string{
				strconv.Itoa(maxOrder),
				"2026",
				zone.Name,
				strconv.Itoa(match.OrderNumber),
				group,
				match.RedSide.Player.Team.CollegeName,
				match.RedSide.Player.Team.Name,
				match.BlueSide.Player.Team.CollegeName,
				match.BlueSide.Player.Team.Name,
				strconv.Itoa(match.RedSideWinGameCount),
				strconv.Itoa(match.BlueSideWinGameCount),
			})
		}
	}

	output, err := os.Create(historyPath)
	if err != nil {
		return 0, fmt.Errorf("create history TSV: %w", err)
	}
	writer := csv.NewWriter(output)
	writer.Comma = '\t'
	if err := writer.WriteAll(result); err != nil {
		_ = output.Close()
		return 0, fmt.Errorf("write history TSV: %w", err)
	}
	if err := output.Close(); err != nil {
		return 0, fmt.Errorf("close history TSV: %w", err)
	}
	return added, nil
}
