# RoboMaster 赛程分析软件

正式环境 https://schedule.scutbot.cn/

这是后端仓库，前端仓库见 https://github.com/scutrobotlab/rm-schedule-ui

除后端技术性介绍外，其余内容都写在前端仓库的 README 中。

## 技术方案

### 依赖工具

- Golang 1.22
- Docker

### 编译方式

直接编译

```bash
go mod tidy
go build -o rm-schedule .
```

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
