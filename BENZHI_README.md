# BENZHI_README

## 项目说明

- 项目：VanceMichael/go-label-canalclear-g01-30
- 项目用途：CanalClear is a PostgreSQL-backed canal-port clearance and lock-passage coordination service. Carrier operators declare voyages and canonical manifests, customs officers open and close inspections, dispatchers reserve lock chambers only after release, and every connected workflow writes a tamper-evident tenant audit chain and reliable outbox event.
- Go 工具链：`golang:1.23`
- 前端工具链：无

## 标准构建、运行和测试命令

进入容器后执行：

```bash
# 编译
cd '/app' && GOTOOLCHAIN=local go build ./...

# 启动
cd '/app' && GOTOOLCHAIN=local go run ./cmd/server

# 测试
cd '/app' && GOTOOLCHAIN=local go test ./...
```

## Docker 构建和进入容器

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-task-166-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-task-166-arm64 linux/arm64
docker run -it benzhi-task-166-amd64:latest
docker run -it --platform linux/arm64 benzhi-task-166-arm64:latest
```

## 题目验证命令

1. 预期退出码 0：`go test ./internal/audit -run ^TestAuditSanitizerRedactsNestedStructuredSecrets$ -count=1`
2. 预期退出码 0：`GOTOOLCHAIN=local go build -buildvcs=false ./... && GOTOOLCHAIN=local go vet ./...`

## Bug 复现

Bug 现象、触发步骤和完整错误信息见 `BUG_REPRO.md`。
