package handler

import (
	"encoding/json"
	"fmt"

	"github.com/kataras/iris/v12"
	"github.com/scutrobotlab/rm-schedule/internal/svc"
)

// StaticZoneSeason 定义同一赛季内已归档赛区的静态快照。
type StaticZoneSeason struct {
	Data        []byte
	ZoneIDs     map[string]struct{}
	ZonePath    []string
	ZoneIDField string
}

// RedirectRouteHandlerParam 定义重定向路由处理器的参数
type RedirectRouteHandlerParam struct {
	Name                string
	CacheControl        string
	OriginalUrl         string
	Static              bool
	SeasonMap           map[string][]byte
	StaticZoneSeasonMap map[string]StaticZoneSeason
	Data                []byte
}

// RedirectRouteHandlerFactory 处理重定向路由的工厂函数
func RedirectRouteHandlerFactory(param RedirectRouteHandlerParam) func(c iris.Context) {
	return func(c iris.Context) {
		season := c.URLParam("season")
		if param.SeasonMap != nil {
			if data, ok := param.SeasonMap[season]; ok {
				c.Header("Cache-Control", "public, max-age=60")
				c.ContentType("application/json")
				_, err := c.Write(data)
				if err != nil {
					_ = c.JSON(iris.Map{"code": -1, "msg": "Failed to get " + param.Name})
				}
				return
			}
		}

		if param.StaticZoneSeasonMap != nil {
			if staticZoneSeason, ok := param.StaticZoneSeasonMap[season]; ok {
				if cached, b := svc.Cache.Get(param.Name); b {
					if cachedData, ok := cached.([]byte); ok {
						if data, err := mergeStaticZones(staticZoneSeason, cachedData); err == nil {
							c.Header("Cache-Control", param.CacheControl)
							c.ContentType("application/json")
							_, err = c.Write(data)
							if err != nil {
								_ = c.JSON(iris.Map{"code": -1, "msg": "Failed to get " + param.Name})
							}
							return
						}
					}
				}

				c.Header("Cache-Control", "public, max-age=60")
				c.ContentType("application/json")
				_, err := c.Write(staticZoneSeason.Data)
				if err != nil {
					_ = c.JSON(iris.Map{"code": -1, "msg": "Failed to get " + param.Name})
				}
				return
			}
		}

		if param.Static {
			c.Header("Cache-Control", "public, max-age=60")
			c.ContentType("application/json")
			_, err := c.Write(param.Data)
			if err != nil {
				_ = c.JSON(iris.Map{"code": -1, "msg": "Failed to get " + param.Name})
			}
			return
		}

		// 是否存在 Tencent-Acceleration-Domain-Name
		if c.GetHeader("Tencent-Acceleration-Domain-Name") != "" {
			c.Header("Cache-Control", param.CacheControl)
			c.Redirect(param.OriginalUrl, 301)
			return
		}

		if cached, b := svc.Cache.Get(param.Name); b {
			c.Header("Cache-Control", param.CacheControl)
			c.ContentType("application/json")
			_, err := c.Write(cached.([]byte))
			if err != nil {
				_ = c.JSON(iris.Map{"code": -1, "msg": "Failed to get " + param.Name})
			}
			return
		}

		c.Header("Cache-Control", param.CacheControl)
		c.StatusCode(500)
		_ = c.JSON(iris.Map{"code": -1, "msg": "Failed to get " + param.Name})
	}
}

func mergeStaticZones(staticZoneSeason StaticZoneSeason, dynamicData []byte) ([]byte, error) {
	var staticRoot map[string]any
	if err := json.Unmarshal(staticZoneSeason.Data, &staticRoot); err != nil {
		return nil, err
	}

	var dynamicRoot map[string]any
	if err := json.Unmarshal(dynamicData, &dynamicRoot); err != nil {
		return nil, err
	}

	staticZones, err := zoneNodes(staticRoot, staticZoneSeason.ZonePath)
	if err != nil {
		return nil, err
	}
	dynamicZones, err := zoneNodes(dynamicRoot, staticZoneSeason.ZonePath)
	if err != nil {
		return nil, err
	}

	mergedZones := make([]any, 0, len(staticZones)+len(dynamicZones))
	for _, zone := range staticZones {
		if _, ok := staticZoneSeason.ZoneIDs[zoneID(zone, staticZoneSeason.ZoneIDField)]; ok {
			mergedZones = append(mergedZones, zone)
		}
	}
	for _, zone := range dynamicZones {
		if _, ok := staticZoneSeason.ZoneIDs[zoneID(zone, staticZoneSeason.ZoneIDField)]; !ok {
			mergedZones = append(mergedZones, zone)
		}
	}

	if err := setZoneNodes(dynamicRoot, staticZoneSeason.ZonePath, mergedZones); err != nil {
		return nil, err
	}

	return json.Marshal(dynamicRoot)
}

func zoneNodes(root map[string]any, path []string) ([]any, error) {
	node, err := zonePathNode(root, path)
	if err != nil {
		return nil, err
	}
	zones, ok := node.([]any)
	if !ok {
		return nil, fmt.Errorf("zone path %v is not an array", path)
	}
	return zones, nil
}

func setZoneNodes(root map[string]any, path []string, zones []any) error {
	if len(path) == 0 {
		return fmt.Errorf("empty zone path")
	}
	parent, err := zonePathNode(root, path[:len(path)-1])
	if err != nil {
		return err
	}
	parentMap, ok := parent.(map[string]any)
	if !ok {
		return fmt.Errorf("zone path parent %v is not an object", path)
	}
	parentMap[path[len(path)-1]] = zones
	return nil
}

func zonePathNode(root map[string]any, path []string) (any, error) {
	var node any = root
	for _, key := range path {
		nodeMap, ok := node.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("zone path %v is not an object", path)
		}
		node = nodeMap[key]
	}
	return node, nil
}

func zoneID(zone any, field string) string {
	zoneMap, ok := zone.(map[string]any)
	if !ok {
		return ""
	}
	id, ok := zoneMap[field].(string)
	if !ok {
		return ""
	}
	return id
}
