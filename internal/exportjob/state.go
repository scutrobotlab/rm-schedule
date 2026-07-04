package exportjob

import (
	"fmt"
	"sync"
	"time"

	"github.com/scutrobotlab/rm-schedule/internal/static"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusReady   Status = "ready"
	StatusError   Status = "error"
)

type PartState struct {
	Index        int
	Name         string
	Type         string
	Group        string
	Status       Status
	ImageURL     string // pending/error 时可能仍指向上一版旧图
	UpdatedAt    time.Time
	ScheduleHash string
	Error        string // 仅内存态，不落盘；重启后随渲染重试自然消失
}

// zoneWatchState 记录单个 zone 的 hash 监听状态（粒度为整个 zone，非单个 part）。
type zoneWatchState struct {
	lastHash     string    // 上次全部 part 渲染成功时对应的 schedule hash
	lastRenderAt time.Time // 上次触发渲染的时间，用于冷却期计算
	pending      bool      // 冷却期内检测到变化，或上次渲染未全部成功
}

type manager struct {
	mu    sync.RWMutex
	parts map[string]*PartState
	zones map[string]*zoneWatchState
	cfg   Config
}

var defaultManager = &manager{
	parts: make(map[string]*PartState),
	zones: make(map[string]*zoneWatchState),
}

func partKey(season, zoneID, partIndex int) string {
	return fmt.Sprintf("%d:%d:%d", season, zoneID, partIndex)
}

func zoneKey(season, zoneID int) string {
	return fmt.Sprintf("%d:%d", season, zoneID)
}

func initManager(cfg Config) {
	defaultManager.mu.Lock()
	defer defaultManager.mu.Unlock()
	defaultManager.cfg = cfg
}

func ensureConfig() Config {
	defaultManager.mu.Lock()
	defer defaultManager.mu.Unlock()
	if defaultManager.cfg.StorageDir == "" {
		defaultManager.cfg = loadConfig()
	}
	return defaultManager.cfg
}

// Get 返回指定 season/zone 下各 part 的状态，与 CurrentSeasonZones 中的 part 清单对齐。
func Get(season, zoneID int) []PartState {
	zone, ok := static.FindCurrentSeasonZone(zoneID)
	if !ok {
		return nil
	}

	defaultManager.mu.RLock()
	defer defaultManager.mu.RUnlock()

	out := make([]PartState, 0, len(zone.Parts))
	for _, part := range zone.Parts {
		key := partKey(season, zoneID, part.Index)
		if st, ok := defaultManager.parts[key]; ok {
			out = append(out, *st)
			continue
		}
		out = append(out, PartState{
			Index:  part.Index,
			Name:   part.Name,
			Type:   part.Type,
			Group:  part.Group,
			Status: StatusPending,
		})
	}
	return out
}

func (m *manager) getOrCreatePart(season, zoneID int, part static.PartManifest) *PartState {
	key := partKey(season, zoneID, part.Index)
	if st, ok := m.parts[key]; ok {
		return st
	}
	st := &PartState{
		Index:  part.Index,
		Name:   part.Name,
		Type:   part.Type,
		Group:  part.Group,
		Status: StatusPending,
	}
	m.parts[key] = st
	return st
}

func (m *manager) setPartReady(season, zoneID int, part static.PartManifest, imageURL, scheduleHash string, updatedAt time.Time) {
	st := m.getOrCreatePart(season, zoneID, part)
	st.Status = StatusReady
	st.ImageURL = imageURL
	st.ScheduleHash = scheduleHash
	st.UpdatedAt = updatedAt
	st.Error = ""
}

func (m *manager) setPartPending(season, zoneID int, part static.PartManifest) {
	st := m.getOrCreatePart(season, zoneID, part)
	// 仅将 ready 降为 pending；保留 ImageURL 以便 manifest 继续返回旧图。
	if st.Status == StatusReady {
		st.Status = StatusPending
	}
	st.Error = ""
}

func (m *manager) setPartError(season, zoneID int, part static.PartManifest, errMsg string) {
	st := m.getOrCreatePart(season, zoneID, part)
	st.Status = StatusError
	st.Error = errMsg // 不清理 ImageURL，调用方可继续展示上一版成功图片
}

func (m *manager) zoneWatch(season, zoneID int) *zoneWatchState {
	key := zoneKey(season, zoneID)
	if zs, ok := m.zones[key]; ok {
		return zs
	}
	zs := &zoneWatchState{}
	m.zones[key] = zs
	return zs
}

func (m *manager) restoreFromMeta(meta MetaFile, imageURL string) {
	zone, ok := static.FindCurrentSeasonZone(meta.ZoneID)
	if !ok {
		return
	}
	var part static.PartManifest
	found := false
	for _, p := range zone.Parts {
		if p.Index == meta.Group {
			part = p
			found = true
			break
		}
	}
	if !found {
		return
	}

	st := m.getOrCreatePart(meta.Season, meta.ZoneID, part)
	st.Status = StatusReady
	st.ImageURL = imageURL
	st.ScheduleHash = meta.ScheduleHash
	st.UpdatedAt = meta.UpdatedAt
	st.Error = ""

	if !meta.Static {
		// 仅非归档赛区恢复 watcher hash 状态，据此判断重启后赛程是否已变化；
		// 归档赛区（static=true）渲染一次后不再监听，其 schedule_hash 仅作版本标识。
		zs := m.zoneWatch(meta.Season, meta.ZoneID)
		if meta.ScheduleHash != "" {
			zs.lastHash = meta.ScheduleHash
		}
		if !meta.UpdatedAt.IsZero() && meta.UpdatedAt.After(zs.lastRenderAt) {
			zs.lastRenderAt = meta.UpdatedAt
		}
	}
}

func isArchivedZone(zoneID int) bool {
	_, ok := static.ArchivedZoneIDs[fmt.Sprintf("%d", zoneID)]
	return ok
}
