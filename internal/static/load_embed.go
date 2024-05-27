package static

import _ "embed"

//go:embed rank_score.json
var RankScoreBytes []byte

//go:embed schedule.json
var ScheduleBytes []byte
