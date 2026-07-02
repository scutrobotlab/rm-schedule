# CLAUDE.md — rm-schedule

## 项目概述

**rm-schedule** 是 RoboMaster 赛程分析软件的后端，由华南虎软件开发组维护。面向 RM 赛事前端提供赛程、分组排名、机器人数据、比赛回放映射等只读 REST API；数据来源为阿里云 OSS 官方 `live_json`、内嵌赛季 JSON 快照与 B 站开放接口，**无传统数据库**。

- 线上地址：<https://schedule.scutbot.cn/>
- 前端仓库：<https://github.com/scutrobotlab/rm-schedule-ui>
- 许可协议：Apache 2.0

---

## 技术栈

| 类别 | 技术 |
|------|------|
| 语言 | Go 1.26 |
| Web 框架 | Iris v12（`github.com/kataras/iris/v12`） |
| 内存缓存 | go-cache v2（`github.com/patrickmn/go-cache`） |
| 定时任务 | robfig/cron v3 |
| 日志 | logrus |
| 工具库 | samber/lo、golang.org/x/text |
| 静态嵌入 | Go `embed` 指令（`//go:embed`） |
| 容器 | Docker 多阶段构建（Go 编译 + alpine 运行，含 Chromium 与 CJK 字体） |
| 无头浏览器 | chromedp（赛程图导出） |
| CI/CD | GitHub Actions → 推送至 GHCR 与阿里云容器镜像服务 |

---

## 项目结构

```
rm-schedule/
├── main.go                        # 入口：启动 cron、Iris，监听 :8080
├── go.mod / go.sum
├── Dockerfile                     # 多阶段：前端 Node 构建 + Go 编译 + alpine 运行
├── .github/workflows/build.yml    # CI：push 触发，构建并推送 Docker 镜像
└── internal/
    ├── common/
    │   ├── upstream.go            # OSS / B 站 URL 常量
    │   └── transparent_to_white.go# PNG 透明底转白底
    ├── handler/
    │   ├── handler_factory.go     # RedirectRouteHandlerFactory（赛程/排名/机器人数据）
    │   ├── rm_static.go           # /api/static/*path 静态资源代理
    │   ├── mp_match.go            # /api/mp/match 小程序投票数据
    │   ├── rank_list.go           # /api/rank 积分榜
    │   ├── bilibili_replay.go     # /api/match_id_to_video、/api/match_order_to_video
    │   ├── team_info.go           # /api/team_info
    │   ├── history_match.go       # /api/history_match
    │   ├── live_json.go           # /api/live_json/*path 反向代理
    │   └── export_image.go        # /api/export_image 赛程图 PNG 导出
    ├── render/
    │   └── schedule_image.go      # chromedp 渲染导出页、singleflight + 15s 缓存
    ├── job/
    │   ├── init.go                # InitCronJob：注册所有 cron 任务
    │   ├── job_factory.go         # CronJobFactory：拉取 OSS JSON 并写入 svc.Cache
    │   └── bilibili/
    │       ├── bilibili_requests.go  # 请求 B 站合集列表 API
    │       └── bilibili_parsers.go   # 解析合集、匹配场次，构建回放映射
    ├── router/
    │   ├── router.go              # 路由注册，托管 ./public 前端
    │   └── redirect.go            # RedirectParams 工厂参数配置
    ├── static/
    │   ├── load_embed.go          # //go:embed 声明所有内嵌 JSON
    │   ├── *.json / *.tsv         # 默认赛季快照（历史交手、积分、完整形态等）
    │   ├── season_2024/           # 2024 赛季快照
    │   └── season_2025/           # 2025 赛季快照
    ├── svc/
    │   └── service_context.go     # 全局 go-cache 单例（svc.Cache）、chromedp allocator
    ├── types/
    │   ├── schedule.go            # 赛程域核心类型：Event/ZoneNode/MatchNode/Side 等
    │   └── bilibili.go            # B 站合集、稿件、回放映射类型
    └── analyze/
        └── schedule.go            # 工具：从 OSS 拉赛程并导出为 CSV
```

---

## 核心功能

- **实时赛程代理**：每 5 秒从阿里云 OSS 拉取 `schedule.json`、`group_rank_info.json`、`robot_data.json`，写入 `svc.Cache`；`schedule` 中的图片 URL 同步改写为 `/api/static/...` 代理路径
- **历史赛季快照**：通过 `?season=2024` / `?season=2025` 查询参数，直接返回编译期嵌入的对应赛季 JSON（`Cache-Control: max-age=60`）
- **B 站回放映射**：每 5 分钟抓取 B 站 UID 20554233 的合集列表，按赛季/赛区/场次号匹配 `MatchNode`，构建 `match_id` 与 `season/zone/order` 两套索引
- **静态资源代理**：`/api/static/*path` 拉取 DJI CDN / 阿里云 / OSS 资源，支持 `?process=bg_white` 将 PNG 透明底转白底，结果写入内存缓存
- **CDN 回源**：请求头携带 `Tencent-Acceleration-Domain-Name` 时，直接 301 重定向到 OSS 原始 URL，减少本机流量
- **小程序投票**：代理 `mp.robomaster.com` 接口，计算红蓝支持比例并短时缓存
- **历史交手查询**：从内嵌 `history_match.json` 按学校/队名检索历史对阵记录
- **赛程图导出**：`/api/export_image` 通过 chromedp 无头浏览器打开前端 `/:season/:zoneId/export` 页面，调用 `relation-graph` 的 `getImageBase64()` 生成 PNG；结果带 15s TTL 缓存与 singleflight 去重，并发渲染上限 3

---

## API 端点

前缀 `/api`，所有端点只读，无鉴权。

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/schedule` | 赛程数据（支持 `?season=` 选历史快照） |
| GET | `/api/group_rank_info` | 小组积分排名（支持 `?season=`） |
| GET | `/api/robot_data` | 机器人统计数据（支持 `?season=`） |
| GET | `/api/rank` | 积分榜与完整形态榜（`?season=`、`?school_name=`） |
| GET | `/api/mp/match` | 小程序对局/预言家数据（`?match_ids=` 逗号分隔） |
| GET | `/api/match_id_to_video` | 比赛 ID → B 站回放元数据（`?match_id=` 或 `all`） |
| GET | `/api/match_order_to_video` | 场次号 → B 站回放元数据（`?season=&zone=&order_number=` 或 `all`） |
| GET | `/api/team_info` | 队伍详情及 B 站官方账号 UID（`?college_name=`） |
| GET | `/api/history_match` | 两队历史对阵（`?primary_college_name=&secondary_college_name=`） |
| GET | `/api/static/*path` | 静态资源代理（可选 `?process=bg_white`） |
| GET | `/api/live_json/*path` | 反向代理 `https://rm-static.djicdn.com/live_json/...` |
| GET | `/api/export_image` | 赛程图 PNG 导出（`?season=&zone=` 必填，`group` 默认 0，`scale` 默认 2） |
| GET | `/` | 托管前端 SPA（`./public`），404 → 302 到 `/` |

---

## 定时任务

在 `main.go` 启动时通过 `job.InitCronJob()` 初始化，**启动时立即执行一轮**：

| 频率 | 任务 | 说明 |
|------|------|------|
| 每 5 秒 | `CronJobFactory` × 3 | 分别拉取 `group_rank_info.json`、`robot_data.json`、`schedule.json`；schedule 还会替换图床域名 |
| 每 5 分钟 | `bilibili.FetchBiliBiliReplayVideos` | 抓取 B 站合集列表，解析场次标题，重建回放映射 |

---

## 数据流与缓存策略

```
阿里云 OSS (live_json)
        │ HTTP GET（每 5s）
        ▼
  job.CronJobFactory
        │ 写入
        ▼
  svc.Cache (go-cache, 无过期)
        │ 读取
        ▼
  handler.RedirectRouteHandlerFactory
        │ 优先级
        ├─ 1. ?season= 命中 SeasonMap → 返回 embed JSON (max-age=60)
        ├─ 2. 请求头有腾讯加速域名 → 301 到 OSS 直链
        └─ 3. 从 Cache 读取 → 返回 JSON / 500

B 站 API (每 5m)
        │
        ▼
  bilibili.FetchBiliBiliReplayVideos
        │ 写入两套索引
        ▼
  svc.Cache: "match_id_to_video" / "match_order_to_video"
```

`/api/static/` 拉取后也写入 `svc.Cache`（key 为路径 + 处理参数），命中直接返回。

---

## 开发与构建命令

**本地编译：**

```bash
go mod tidy
go build -o rm-schedule .
./rm-schedule        # 监听 :8080，./public 为前端目录
```

**本地无 Docker 调试（赛程图导出）：**

`/api/export_image` 需要 chromedp 打开前端导出页。本地 `frontend` 子模块通常为空，需并行启动 `rm-schedule-ui` 的 vite dev server，并把 `SCHEDULE_RENDER_BASE_URL` 指向它：

```bash
# 终端 A
cd rm-schedule-ui && yarn dev    # :3000，/api 代理到 :8080

# 终端 B
cd rm-schedule
SCHEDULE_RENDER_BASE_URL=http://localhost:3000 go run .

# 验证
curl "http://localhost:8080/api/export_image?season=2026&zone=616&group=0" -o out.png
```

本机需已安装 Chrome 或 Chromium（macOS 下 chromedp 自动探测，无需 `CHROME_PATH`）。Docker/生产环境默认 `SCHEDULE_RENDER_BASE_URL=http://127.0.0.1:8080`，容器内已安装 `chromium` 与 `font-noto-cjk` 字体。

**构建 Docker 镜像**（Dockerfile 依赖 `./frontend` 子目录存放前端源码）：

```bash
# 先将 rm-schedule-ui 克隆到 ./frontend
git clone https://github.com/scutrobotlab/rm-schedule-ui frontend

docker build --platform linux/amd64 \
  -t registry.cn-guangzhou.aliyuncs.com/scutrobot/rm-schedule:latest .
```

**推送镜像：**

```bash
docker push registry.cn-guangzhou.aliyuncs.com/scutrobot/rm-schedule:latest
```

**CI（GitHub Actions）**：`push` 时自动构建并推送至：
- `ghcr.io/scutrobotlab/rm-schedule:latest`（及 SHA、分支名 tag）
- `registry.cn-guangzhou.aliyuncs.com/scutrobot/rm-schedule:latest`

---

## 注意事项

- **无数据库**：所有业务数据通过 `go-cache` 内存缓存 + `//go:embed` 编译期快照提供，无 ORM/SQL。
- **无鉴权**：所有 `/api/*` 端点公开只读，无 JWT/Session/RBAC。
- **嵌入赛季快照管理**：新增赛季时需在 `internal/static/season_XXXX/` 放置 JSON 文件，并在 `load_embed.go` 中补充 `//go:embed` 声明，在 `router/redirect.go` 中更新 `SeasonMap`。
- **B 站解析特殊规则**：
  - 合集标题匹配依赖关键词"RMUC/超级对抗赛 + 回放 + 赛季 + 赛区名"；
  - 港澳台等长赛区名与 B 站标题用前 3 个 rune 做模糊匹配；
  - 2025 "复活赛第一赛段" 有硬编码合集 ID。
- **前端在 Docker 内构建**：Dockerfile 的前端阶段用 `yarn` 构建 `./frontend`（CI 通过 `actions/checkout` 将 `rm-schedule-ui` 克隆到该路径），本地构建需手动准备。
- **无测试框架**：仅有 `internal/static/convert_test.go`（JSON 转换工具测试）与 `internal/analyze/schedule_test.go`、`internal/common/transparent_to_white_test.go`，无集成测试。
- **schedule 图床替换**：`CronJobFactory` 对 schedule JSON 做字符串替换，将 DJI/阿里云等图床域名统一改写为 `/api/static/...`，依赖 `internal/common` 中定义的域名列表。
