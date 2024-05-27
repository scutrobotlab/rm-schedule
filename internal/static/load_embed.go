package static

import _ "embed"

//go:embed rank_list.json
var RankListBytes []byte

//go:embed schedule.json
var ScheduleBytes []byte
