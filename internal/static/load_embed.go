package static

import _ "embed"

//go:embed complete_form.json
var CompleteFormBytes []byte

//go:embed rank_score.json
var RankScoreBytes []byte

//go:embed schedule.json
var ScheduleBytes []byte
