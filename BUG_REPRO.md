# Bug Reproduction

## 包的性质

当前 test_model_fix 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。要复现原始缺陷，必须检出下面固定的 parent SHA；不要在当前修复结果源码上期待重新出现修复前失败。生成系统使用的可信验证补丁和完整验证日志仅在本地留存，不提交到结果分支。

## 问题现象

审计检索接口已经会遮住 details 顶层的 token，但海关申报把请求快照作为 JSON 字符串放进去后，credentials 对象和 attempts 数组里的 access_token、session、Authorization 仍原样返回，审计员导出记录时能直接看到凭证。请修复结构化详情的脱敏，递归处理对象和数组，同时保留普通字段、无效 JSON 原文和原始审计链数据。

## 含 Bug 版本

- 仓库：VanceMichael/go-label-canalclear-g01-30
- 仓库地址：https://github.com/VanceMichael/go-label-canalclear-g01-30.git
- parent SHA：78683aa24dc9af4254fdcbdb61b20cd8bae2c154

## 复现步骤

```bash
git clone -- https://github.com/VanceMichael/go-label-canalclear-g01-30.git bug-repro
cd bug-repro
git checkout --detach 78683aa24dc9af4254fdcbdb61b20cd8bae2c154
go test ./internal/audit -run ^TestAuditSanitizerRedactsNestedStructuredSecrets$ -count=1
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/audit -run ^TestAuditSanitizerRedactsNestedStructuredSecrets$ -count=1
--- FAIL: TestAuditSanitizerRedactsNestedStructuredSecrets (0.00s)
    detail_sanitizer_test.go:18: nested secret "live-token" remains in sanitized request: {"route":"customs.submit","credentials":{"access_token":"live-token","session":"session-42"},"attempts":[{"authorization":"Bearer live-token"}]}
FAIL
FAIL	github.com/VanceMichael/go-base-canalclear-g01/internal/audit	0.031s
FAIL

```

stderr：

```text
(empty)
```

### linux/arm64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/audit -run ^TestAuditSanitizerRedactsNestedStructuredSecrets$ -count=1
--- FAIL: TestAuditSanitizerRedactsNestedStructuredSecrets (0.00s)
    detail_sanitizer_test.go:18: nested secret "live-token" remains in sanitized request: {"route":"customs.submit","credentials":{"access_token":"live-token","session":"session-42"},"attempts":[{"authorization":"Bearer live-token"}]}
FAIL
FAIL	github.com/VanceMichael/go-base-canalclear-g01/internal/audit	0.002s
FAIL

```

stderr：

```text
(empty)
```

## 通过条件

审计详情中的结构化 JSON 无论凭证键位于多少层对象或数组，都必须按既有敏感键规则替换为 [redacted]，响应不得包含 access token、会话值或 Authorization 内容；非敏感字段、无法解析的普通文本以及存储中的原始 Event.Details 和哈希链保持不变。go test ./internal/audit -run ^TestAuditSanitizerRedactsNestedStructuredSecrets$ -count=1 必须由红转绿，audit 与 HTTP 查询回归、全仓 go test ./...、go build ./... 均通过；不得删除结构化快照、整体抹除 details 或弱化三个嵌套凭证断言。
