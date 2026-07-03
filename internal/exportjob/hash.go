package exportjob

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
)

func zoneHashFromSchedule(scheduleData []byte, zoneID int) (string, error) {
	zoneNode, err := extractZoneNode(scheduleData, zoneID)
	if err != nil {
		return "", err
	}
	if zoneNode == nil {
		return "", fmt.Errorf("zone %d not found in schedule", zoneID)
	}

	payload := make(map[string]any, 3)
	for _, key := range []string{"groups", "groupMatches", "knockoutMatches"} {
		if v, ok := zoneNode[key]; ok {
			payload[key] = v
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal zone subtree: %w", err)
	}

	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func extractZoneNode(scheduleData []byte, zoneID int) (map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(scheduleData, &root); err != nil {
		return nil, fmt.Errorf("unmarshal schedule: %w", err)
	}

	nodes, err := zoneNodesAtPath(root, []string{"data", "event", "zones", "nodes"})
	if err != nil {
		return nil, err
	}

	targetID := strconv.Itoa(zoneID)
	for _, node := range nodes {
		zoneMap, ok := node.(map[string]any)
		if !ok {
			continue
		}
		id, _ := zoneMap["id"].(string)
		if id == targetID {
			return zoneMap, nil
		}
	}
	return nil, nil
}

func zoneNodesAtPath(root map[string]any, path []string) ([]any, error) {
	var node any = root
	for _, key := range path {
		nodeMap, ok := node.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %v is not an object", path)
		}
		node = nodeMap[key]
	}
	nodes, ok := node.([]any)
	if !ok {
		return nil, fmt.Errorf("path %v is not an array", path)
	}
	return nodes, nil
}
