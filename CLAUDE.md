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
├── main.go                        # 入口：Chrome、exportjob bootstrap、cron、Iris，监听 :8080
├── go.mod / go.sum
├── Dockerfile                     # 多阶段：前端 Node 构建 + Go 编译 + alpine 运行
├── .github/workflows/build.yml    # CI：push 触发，构建并推送 Docker 镜像
└── internal/
    ├── common/
    │   ├── upstream.go            # OSS / B 站 URL 常量
    │   └── transparent_to_white.go# PNG 透明底转白底
    ├── exportjob/
    │   ├── bootstrap.go           # 启动时扫描磁盘 meta 恢复状态，异步渲染归档赛区
    │   ├── watcher.go             # CheckAndRender：每 5s 对比 schedule hash 触发后台渲染
    │   ├── state.go               # 内存状态表（pending/ready/error）与查询接口
    │   ├── render_zone.go         # 按 zone/part 调用 render + storage 落盘
    │   ├── meta.go                # 与图片同名的 .meta.json 读写
    │   ├── hash.go                # 从 schedule JSON 提取 zone 子树并算 sha256
    │   └── config.go              # SCHEDULE_EXPORT_* 环境变量读取
    ├── handler/
    │   ├── handler_factory.go     # RedirectRouteHandlerFactory（赛程/排名/机器人数据）
    │   ├── rm_static.go           # /api/static/*path 静态资源代理
    │   ├── mp_match.go            # /api/mp/match 小程序投票数据
    │   ├── rank_list.go           # /api/rank 积分榜
    │   ├── bilibili_replay.go     # /api/match_id_to_video、/api/match_order_to_video
    │   ├── team_info.go           # /api/team_info
    │   ├── history_match.go       # /api/history_match
    │   ├── live_json.go           # /api/live_json/*path 反向代理
    │   ├── export_image.go        # /api/export_image 赛程图 PNG 同步导出（15s 缓存）
    │   └── export_manifest.go     # /api/export_manifest 当前赛季导出图清单与状态
    ├── render/
    │   └── schedule_image.go      # chromedp RenderOnce + HTTP 路径 singleflight + 15s 缓存
    ├── storage/
    │   ├── store.go               # Store 接口与 NewStoreFromEnv 工厂
    │   ├── local.go               # LocalStore：写入 SCHEDULE_EXPORT_STORAGE_DIR
    │   └── cos.go                 # CosStore 骨架（当前未实现）
    ├── job/
    │   ├── init.go                # InitCronJob：注册 OSS 拉取与 B 站任务
    │   ├── job_factory.go         # CronJobFactory：拉取 OSS JSON 并写入 svc.Cache
    │   └── bilibili/
    │       ├── bilibili_requests.go  # 请求 B 站合集列表 API
    │       └── bilibili_parsers.go   # 解析合集、匹配场次，构建回放映射
    ├── router/
    │   ├── router.go              # 路由注册，托管 ./public 前端与 /api/export_static
    │   └── redirect.go            # RedirectParams 工厂参数配置
    ├── static/
    │   ├── load_embed.go          # //go:embed 声明所有内嵌 JSON
    │   ├── season_manifest.go     # CurrentSeasonZones / ArchivedZoneIDs（需与前端 zone.ts 同步）
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
- **赛程图导出（同步）**：`/api/export_image` 通过 chromedp 无头浏览器打开前端 `/:season/:zoneId/export` 页面，调用 `relation-graph` 的 `getImageBase64()` 生成 PNG；结果带 15s TTL 缓存与 singleflight 去重，并发渲染上限 3；适用于历史赛季、归档赛区或手动调试
- **赛程图后台导出（当前赛季）**：`exportjob.Watcher` 每 5 秒读取 `svc.Cache["schedule"]`，按 zone 子树 hash 检测变化，冷却期（默认 20s）过后触发 chromedp 渲染并落盘至 `SCHEDULE_EXPORT_STORAGE_DIR`；`/api/export_manifest` 返回各 part 的 `status`/`image_url`；`/api/export_static/` 托管本地图片；归档赛区（614/615/616）渲染一次后永久保留、不监听变化，但仍未 ready 的 part（如 bootstrap 重试耗尽）由 cron 按冷却期节奏长期兜底补渲染，成功后不再重试

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
| GET | `/api/export_image` | 赛程图 PNG 同步导出（`?season=&zone=` 必填，`group` 默认 0，`scale` 默认 2） |
| GET | `/api/export_manifest` | 当前赛季导出图清单（`?season=&zone=` 必填；仅支持 `static.CurrentSeason`，否则 400） |
| GET | `/api/export_static/*path` | 后台导出的赛程图 PNG（`local` 存储后端时由 Iris 静态托管；URL 带 `?v=` 做 cache busting） |
| GET | `/` | 托管前端 SPA（`./public`），404 → 302 到 `/` |

**`/api/export_manifest` 响应示例**（`season=2026&zone=616`）：

```json
{
  "season": 2026,
  "zone": { "id": 616, "name": "北部赛区" },
  "groups": [
    {
      "index": 0,
      "name": "A组前段",
      "type": "group",
      "group": "A",
      "status": "ready",
      "image_url": "/api/export_static/2026/616/0.png?v=1730000000",
      "updated_at": "2026-07-03T12:00:00+08:00"
    }
  ]
}
```

`status` 取值：`pending`（等待/冷却中）、`ready`（已有可用图片）、`error`（最近一次渲染失败，错误信息仅存内存）。`pending` 时 `image_url` 可能为空或指向上一版旧图。

---

## 定时任务

在 `main.go` 启动时初始化 storage、`exportjob.Bootstrap`，并通过 `job.InitCronJob()` 注册 OSS/B 站任务；**启动时立即执行一轮**：

| 频率 | 任务 | 说明 |
|------|------|------|
| 每 5 秒 | `CronJobFactory` × 3 | 分别拉取 `group_rank_info.json`、`robot_data.json`、`schedule.json`；schedule 还会替换图床域名 |
| 每 5 秒 | `exportjob.CheckAndRender` | 对比当前赛季非归档 zone 的 schedule hash，冷却期过后后台渲染并落盘 |
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

exportjob.CheckAndRender (每 5s，SCHEDULE_EXPORT_ENABLED=true)
        │ 读取 schedule hash
        ▼
  render.RenderOnce + storage.LocalStore
        │ 写入图片 + .meta.json
        ▼
  exportjob 内存状态表 ──► GET /api/export_manifest
        │
        └──► GET /api/export_static/2026/{zone}/{part}.png
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

## 环境变量

### 赛程图同步导出（`/api/export_image`）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SCHEDULE_RENDER_BASE_URL` | `http://127.0.0.1:8080` | chromedp 打开的前端导出页根 URL |

### 赛程图后台导出（`exportjob` / `/api/export_manifest`）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SCHEDULE_EXPORT_ENABLED` | `true` | 总开关；`false` 时跳过 bootstrap 与 watcher |
| `SCHEDULE_EXPORT_STORAGE_BACKEND` | `local` | `local` \| `cos`（后者当前为占位实现，调用会返回未实现错误） |
| `SCHEDULE_EXPORT_STORAGE_DIR` | `./data/export_images` | 本地存储目录；容器内需挂载持久化卷 |
| `SCHEDULE_EXPORT_PUBLIC_BASE_URL` | `""`（相对路径） | 拼接 `image_url` 的域名前缀，如 `https://schedule.scutbot.cn` |
| `SCHEDULE_EXPORT_RENDER_COOLDOWN` | `20s` | 同一 zone 两次后台渲染之间的最小间隔 |
| `SCHEDULE_EXPORT_SCALE` | `2` | 后台渲染使用的 `scale` 参数（1–8） |
| `SCHEDULE_EXPORT_RENDER_MAX_ATTEMPTS` | `3` | 归档赛区单个 part 渲染的最大尝试次数（含首次），瞬时错误退避重试 |
| `SCHEDULE_RENDER_READY_TIMEOUT` | `60s` | Bootstrap 渲染前等待渲染目标就绪的最长时间；`0` 表示不等待 |
| `SCHEDULE_EXPORT_COS_BUCKET` | 空 | 预留腾讯云 COS 配置 |
| `SCHEDULE_EXPORT_COS_REGION` | 空 | 预留 |
| `SCHEDULE_EXPORT_COS_SECRET_ID` | 空 | 预留 |
| `SCHEDULE_EXPORT_COS_SECRET_KEY` | 空 | 预留 |
| `SCHEDULE_EXPORT_COS_DOMAIN` | 空 | 预留 |

**本地验证后台导出：**

```bash
# 启动后等待 watcher 渲染，或查询 manifest 状态
curl "http://localhost:8080/api/export_manifest?season=2026&zone=616"
curl "http://localhost:8080/api/export_static/2026/616/0.png" -o out.png
```

---

## 注意事项

- **无数据库**：所有业务数据通过 `go-cache` 内存缓存 + `//go:embed` 编译期快照提供，无 ORM/SQL。
- **无鉴权**：所有 `/api/*` 端点公开只读，无 JWT/Session/RBAC。
- **嵌入赛季快照管理**：新增赛季时需在 `internal/static/season_XXXX/` 放置 JSON 文件，并在 `load_embed.go` 中补充 `//go:embed` 声明，在 `router/redirect.go` 中更新 `SeasonMap`。
- **当前赛季导出清单维护**：`internal/static/season_manifest.go` 中的 `CurrentSeasonZones`、`ArchivedZoneIDs` 与 `CurrentSeason` 需与前端 `rm-schedule-ui/src/constant/zone.ts` 中 `ZoneMap[2026]` **手工同步**；赛季推进（新增赛区/分组）或赛季切换（如 2027 开赛）时需同步更新该文件，并调整 `export_manifest` 的赛季校验逻辑。
- **后台导出持久化**：`SCHEDULE_EXPORT_STORAGE_DIR` 下每张 PNG 对应同名 `.meta.json`；容器部署时需挂载该目录，否则重启后需重新渲染。
- **归档赛区渲染重试**：归档赛区（614/615/616）由 `Bootstrap` 一次性渲染，故渲染前会先探测 `SCHEDULE_RENDER_BASE_URL` 就绪（避免默认目标即本进程 `:8080` 在 `main.go` 末尾才 `Listen` 引发的启动竞态 `ERR_CONNECTION_REFUSED`），并对瞬时错误（页面加载失败/超时）按 `SCHEDULE_EXPORT_RENDER_MAX_ATTEMPTS` 退避重试；`ParamError`（参数错误）与存储/meta 错误不重试。若 bootstrap 的有界重试仍全部耗尽（如目标长期不可达），`CheckAndRender` 会作为长期兜底，按 `SCHEDULE_EXPORT_RENDER_COOLDOWN` 节奏对未 ready 的归档 part 单次补渲染，成功后永久保留、不再重试。非归档赛区仍由 cron 每 5s + 冷却期自愈。
- **B 站解析特殊规则**：
  - 合集标题匹配依赖关键词"RMUC/超级对抗赛 + 回放 + 赛季 + 赛区名"；
  - 港澳台等长赛区名与 B 站标题用前 3 个 rune 做模糊匹配；
  - 2025 "复活赛第一赛段" 有硬编码合集 ID。
- **前端在 Docker 内构建**：Dockerfile 的前端阶段用 `yarn` 构建 `./frontend`（CI 通过 `actions/checkout` 将 `rm-schedule-ui` 克隆到该路径），本地构建需手动准备。
- **无测试框架**：仅有 `internal/static/convert_test.go`（JSON 转换工具测试）与 `internal/analyze/schedule_test.go`、`internal/common/transparent_to_white_test.go`，无集成测试。
- **schedule 图床替换**：`CronJobFactory` 对 schedule JSON 做字符串替换，将 DJI/阿里云等图床域名统一改写为 `/api/static/...`，依赖 `internal/common` 中定义的域名列表。
