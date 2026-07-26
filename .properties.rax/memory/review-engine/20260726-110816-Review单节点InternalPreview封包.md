# Review单节点 Internal Preview 封包

## 事件

2026-07-26，依据既有 Review design/plan，将已完成的 owner-local 单机软件收口为可复现、可启动、可测试的 Internal Preview；未扩展跨 Owner 合同或 production 语义。

## 产物

- `ExecutionRuntime/review/INTERNAL_PREVIEW.md`：封包能力、启动方法和 NO-GO；
- `scripts/package-internal-preview.sh`：确定性 Linux/amd64 二进制、tar.gz 与 SHA-256；
- `scripts/run-local.sh`：要求调用方显式注入 SQLite 路径、Token 与 cursor key，不生成、不持久化、不输出 Secret；
- `scripts/verify-internal-preview.sh`：校验归档并启动真实 `review-service`，通过真实 SQLite、HTTP 和 CLI 验证；
- `scripts/test-internal-preview.sh`：双构建字节一致、归档篡改与非法 Secret 故障门禁。

## 当前真值

- Review owner-local 单节点 Internal Preview 封包已实现并通过本轮软件门禁，等待提交与 Draft PR；
- 封包继续固定 `reference_only`、`production_eligible=false`；
- Binding/Evidence/Policy/Authority/Scope production adapter/certification、宿主 composition root、真实 Auto Reviewer/外部平台 Provider、OIDC、HA、备份与 SLA 仍 NO-GO；
- Internal Preview 不是 Praxis production integration，也不授 Runtime Authorization、Permit、Begin 或 Provider 调用权。

## 验证

- 两次独立开发态封包构建字节一致；
- 真实 `review-service` 使用临时 SQLite 启动，未认证请求返回 401，封包内 CLI 经 HTTP 成功读取空 Case 集合；
- checksum 篡改与非法 Secret 均失败关闭；
- `go test ./... -count=1`：PASS；
- `go test -race ./... -count=1`：PASS；
- `go vet ./...`：PASS；
- `go test ./tests ./tests/runtimeintegration -run 'Blackbox|Fault' -count=100`：PASS；
- `go test -race ./tests ./tests/runtimeintegration -run 'Blackbox|Fault' -count=20`：PASS；
- `go test -bench 'SQLite|Router|HTTP' -benchmem -count=3 ./...`：PASS，仅记录本机基线，不作 SLA；
- `gofmt -l`、`go mod tidy -diff`、shell syntax/import/diff 检查：PASS；
- 提交后干净工作树双构建归档字节一致，真实服务验证：PASS；归档摘要随最终源码 revision 生成并由同目录 `.sha256` 文件校验。
