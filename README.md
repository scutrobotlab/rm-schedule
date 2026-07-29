# RoboMaster 赛程分析软件

正式环境 https://schedule.scutbot.cn/

这是后端仓库，前端仓库见 https://github.com/scutrobotlab/rm-schedule-ui

除后端技术性介绍外，其余内容都写在前端仓库的 README 中。

## 技术方案

### 依赖工具

- Golang 1.26
- Docker

### 编译方式

直接编译

```bash
go mod tidy
go build -o rm-schedule .
```

### 本地无 Docker 调试（赛程图导出）

赛程图导出接口 `/api/export_image` 依赖 chromedp 无头浏览器渲染前端导出页。本地开发时 `frontend` 子模块通常为空、`./public` 也无现成产物，需同时启动前端 dev server 与后端，并把 chromedp 的目标地址指向前端：

```bash
# 终端 A：前端 dev server（/api 已代理到 :8080）
cd rm-schedule-ui && yarn dev

# 终端 B：后端，chromedp 访问 vite dev server
cd rm-schedule
SCHEDULE_RENDER_BASE_URL=http://localhost:3000 go run .
```

前提：本机已安装 Google Chrome 或 Chromium（macOS 下 chromedp 会自动探测，无需额外配置）。

验证导出：

```bash
curl "http://localhost:8080/api/export_image?season=2026&zone=616&group=0" -o out.png
```

可选参数：`group`（默认 `0`，对应 `zone.parts` 下标）、`scale`（默认 `2`，设备像素比）。

生产/Docker 环境默认 `SCHEDULE_RENDER_BASE_URL=http://127.0.0.1:8080`，chromedp 直接访问容器内托管的前端静态资源，无需额外配置。

测试环境使用与正式环境相同的镜像，并通过运行时环境变量启用页面标记：

```bash
SCHEDULE_IS_TEST_ENVIRONMENT=true
```

后端通过 `GET /api/config` 将该标识下发给前端。正式环境不设置此变量（或设置为 `false`）时不显示标记。

移动端 Bracket UI 支持按浏览器稳定分桶灰度：

```bash
# 数字 0-100，支持 0.01% 精度；默认 0，降低比例可撤销对应范围的灰度
SCHEDULE_MOBILE_BRACKET_ROLLOUT=12.34

# 用于签名 bracket_ui_v1 分桶 Cookie，生产环境建议配置随机密钥
SCHEDULE_EXPERIMENT_COOKIE_SECRET=replace-with-a-random-secret
```

未配置签名密钥时服务会打印错误日志，并使用内置默认值继续运行。命中灰度的移动端在普通赛程路径直接显示 Bracket UI；显式 `/bracket` 路径不受灰度影响。

构建 Docker 镜像

```bash
docker build --platform linux/amd64 -t registry.cn-guangzhou.aliyuncs.com/scutrobot/rm-schedule:latest .
```

推送 Docker 镜像

```bash
docker push registry.cn-guangzhou.aliyuncs.com/scutrobot/rm-schedule:latest
```

### 目录结构

```text
.
├── Dockerfile
├── LICENSE
├── README.md
├── etc 配置文件
├── go.mod
├── go.sum
├── internal 内部代码
│ ├── handler 请求处理器
│ │ ├── ...
│ ├── job 定时任务
│ │ ├── ...
│ ├── router 路由
│ │ └── router.go
│ ├── static 静态资源
│ │ ├── ...
│ └── svc 服务
│ └── service_context.go
└── main.go 主方法
```
