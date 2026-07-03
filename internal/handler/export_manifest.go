package handler

import (
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/exportjob"
	"github.com/scutrobotlab/rm-schedule/internal/static"
)

type exportManifestZone struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type exportManifestGroup struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Group     string `json:"group"`
	Status    string `json:"status"`
	ImageURL  string `json:"image_url,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type exportManifestResponse struct {
	Season int                   `json:"season"`
	Zone   exportManifestZone    `json:"zone"`
	Groups []exportManifestGroup `json:"groups"`
}

func ExportManifestHandler(c iris.Context) {
	c.Header("Cache-Control", "no-store")

	seasonStr := c.URLParam("season")
	zoneStr := c.URLParam("zone")
	if seasonStr == "" || zoneStr == "" {
		c.StatusCode(400)
		_, _ = c.WriteString("season and zone are required")
		return
	}

	season, err := strconv.Atoi(seasonStr)
	if err != nil {
		c.StatusCode(400)
		_, _ = c.WriteString("season must be an integer")
		return
	}
	if season != static.CurrentSeason {
		c.StatusCode(400)
		_, _ = c.WriteString("season not supported")
		return
	}

	zoneID, err := strconv.Atoi(zoneStr)
	if err != nil {
		c.StatusCode(400)
		_, _ = c.WriteString("zone must be an integer")
		return
	}

	zone, ok := static.FindCurrentSeasonZone(zoneID)
	if !ok {
		c.StatusCode(404)
		_, _ = c.WriteString("zone not found")
		return
	}

	parts := exportjob.Get(season, zoneID)
	groups := make([]exportManifestGroup, 0, len(parts))
	for _, part := range parts {
		item := exportManifestGroup{
			Index:    part.Index,
			Name:     part.Name,
			Type:     part.Type,
			Group:    part.Group,
			Status:   string(part.Status),
			ImageURL: part.ImageURL,
		}
		if !part.UpdatedAt.IsZero() {
			item.UpdatedAt = part.UpdatedAt.Format(time.RFC3339)
		}
		groups = append(groups, item)
	}

	_ = c.JSON(exportManifestResponse{
		Season: season,
		Zone: exportManifestZone{
			ID:   zone.ID,
			Name: zone.Name,
		},
		Groups: groups,
	})
}
