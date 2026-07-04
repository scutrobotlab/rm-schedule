// 本文件需与 rm-schedule-ui/src/constant/zone.ts 中 ZoneMap[2026] 手工同步，
// 赛季推进新增赛区/分组时需要同步更新。
// 赛季切换（如 2027 开赛）时，除下方 CurrentSeason / CurrentSeasonZones / ArchivedZoneIDs 外，
// 还需将 CurrentSeasonScheduleBytes 指向对应赛季的内嵌快照。
package static

const CurrentSeason = 2026

// CurrentSeasonScheduleBytes 当前赛季内嵌赛程快照，随 CurrentSeason 同步。
// 供后台导出计算归档赛区的赛程版本 hash 使用，避免在别处硬编码 ScheduleBytes20XX。
var CurrentSeasonScheduleBytes = ScheduleBytes2026

// PartManifest 描述一个赛区下的单张导出图（对应前端 Zone.parts 的一项）。
type PartManifest struct {
	Index int    // 与前端 parts 数组下标一致，也是 export?group= 参数及存储 key 后缀（如 2026/616/0.png）
	Name  string // "A组前段" 等，与前端 zone.ts 保持一致
	Type  string // "group" | "knockout"
	Group string // "A"/"B"/"Knockout" 等
}

// ZoneManifest 描述当前赛季的一个赛区及其全部导出 part。
type ZoneManifest struct {
	ID    int
	Name  string
	Parts []PartManifest
}

// ArchivedZoneIDs 2026 赛季已归档的 regional 赛区 ID。
// key 为 string 是为兼容 handler.StaticZoneSeason.ZoneIDs；这些赛区赛程已定格，
// 后台导出对其「渲染一次、永久保留」，不再持续监听 hash 变化。
var ArchivedZoneIDs = map[string]struct{}{
	"614": {},
	"615": {},
	"616": {},
}

// CurrentSeasonZones 当前赛季需后台导出的赛区/part 清单，供 export_manifest 与 watcher 使用。
var CurrentSeasonZones = []ZoneManifest{
	{
		ID: 614, Name: "南部赛区",
		Parts: []PartManifest{
			{Index: 0, Name: "A组前段", Type: "group", Group: "A"},
			{Index: 1, Name: "B组前段", Type: "group", Group: "B"},
			{Index: 2, Name: "A组后段", Type: "group", Group: "A"},
			{Index: 3, Name: "B组后段", Type: "group", Group: "B"},
			{Index: 4, Name: "全国赛名额争夺战", Type: "group", Group: "Knockout"},
			{Index: 5, Name: "淘汰赛", Type: "knockout", Group: "Knockout"},
		},
	},
	{
		ID: 615, Name: "东部赛区",
		Parts: []PartManifest{
			{Index: 0, Name: "A组前段", Type: "group", Group: "A"},
			{Index: 1, Name: "B组前段", Type: "group", Group: "B"},
			{Index: 2, Name: "A组后段", Type: "group", Group: "A"},
			{Index: 3, Name: "B组后段", Type: "group", Group: "B"},
			{Index: 4, Name: "复活赛名额争夺战", Type: "group", Group: "Knockout"},
			{Index: 5, Name: "淘汰赛", Type: "knockout", Group: "Knockout"},
		},
	},
	{
		ID: 616, Name: "北部赛区",
		Parts: []PartManifest{
			{Index: 0, Name: "A组前段", Type: "group", Group: "A"},
			{Index: 1, Name: "B组前段", Type: "group", Group: "B"},
			{Index: 2, Name: "A组后段", Type: "group", Group: "A"},
			{Index: 3, Name: "B组后段", Type: "group", Group: "B"},
			{Index: 4, Name: "全国赛名额争夺战", Type: "group", Group: "Knockout"},
			{Index: 5, Name: "淘汰赛", Type: "knockout", Group: "Knockout"},
		},
	},
}

// FindCurrentSeasonZone 按 ID 查找当前赛季赛区，找不到时 ok 为 false。
func FindCurrentSeasonZone(zoneID int) (ZoneManifest, bool) {
	for _, z := range CurrentSeasonZones {
		if z.ID == zoneID {
			return z, true
		}
	}
	return ZoneManifest{}, false
}
