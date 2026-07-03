// 本文件需与 rm-schedule-ui/src/constant/zone.ts 中 ZoneMap[2026] 手工同步，
// 赛季推进新增赛区/分组时需要同步更新。
package static

const CurrentSeason = 2026

type PartManifest struct {
	Index int
	Name  string // "A组前段" 等，与前端 zone.ts 保持一致
	Type  string // "group" | "knockout"
	Group string // "A"/"B"/"Knockout" 等
}

type ZoneManifest struct {
	ID    int
	Name  string
	Parts []PartManifest
}

// ArchivedZoneIDs 2026 赛季已归档的 regional 赛区 ID，供静态快照合并与后台导出共用。
var ArchivedZoneIDs = map[string]struct{}{
	"614": {},
	"615": {},
	"616": {},
}

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
