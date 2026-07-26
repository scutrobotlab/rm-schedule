package static

import _ "embed"

// 公共的静态文件

//go:embed complete_form.json
var CompleteFormBytes []byte

//go:embed complete_form_rank.json
var CompleteFormRankBytes []byte

//go:embed rank_score.json
var RankScoreBytes []byte

//go:embed schedule.json
var ScheduleBytes []byte

//go:embed group_rank_info.json
var GroupRankInfoBytes []byte

//go:embed robot_data.json
var RobotDataBytes []byte

//go:embed bilibili_official.json
var BilibiliOfficialBytes []byte

//go:embed history_match.json
var HistoryMatchBytes []byte

//go:embed bilibili_videos.json
var BilibiliVideosBytes []byte

//go:embed team_abbreviation.csv
var TeamAbbreviationBytes []byte

// 2025 赛季的静态文件

//go:embed season_2025/complete_form.json
var CompleteFormBytes2025 []byte

//go:embed season_2025/complete_form_rank.json
var CompleteFormRankBytes2025 []byte

//go:embed season_2025/rank_score.json
var RankScoreBytes2025 []byte

//go:embed season_2025/schedule.json
var ScheduleBytes2025 []byte

//go:embed season_2025/group_rank_info.json
var GroupRankInfoBytes2025 []byte

//go:embed season_2025/robot_data.json
var RobotDataBytes2025 []byte

//go:embed season_2025/bilibili_videos.json
var BilibiliVideosBytes2025 []byte

// 2026 赛季的静态文件

//go:embed season_2026/schedule.json
var ScheduleBytes2026 []byte

//go:embed season_2026/group_rank_info.json
var GroupRankInfoBytes2026 []byte

//go:embed season_2026/robot_data.json
var RobotDataBytes2026 []byte

// 2024 赛季的静态文件

//go:embed season_2024/complete_form.json
var CompleteFormBytes2024 []byte

//go:embed season_2024/rank_score.json
var RankScoreBytes2024 []byte

//go:embed season_2024/schedule.json
var ScheduleBytes2024 []byte

//go:embed season_2024/group_rank_info.json
var GroupRankInfoBytes2024 []byte

//go:embed season_2024/bilibili_videos.json
var BilibiliVideosBytes2024 []byte
