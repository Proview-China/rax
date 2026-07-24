# Application G6A单Call协调闭环完成

时间：2026-07-16 08:46（Asia/Shanghai）

## 本次闭合

- 新增Application自有`SingleCallToolActionRequestV1/ResultV1`及窄Ports。Request只使用Application中立坐标与Runtime公共typed refs，完整绑定Workflow、ExecutionScope、Run、Session、Turn、PendingAction、Model Projection、Assembly、Authority、ParentFrame metadata；Session、Turn、ParentFrame CTX-D10 applicability source为三种不同nominal type和canonical domain；
- Request/Result/Input Projection/Coordination Fact均采用确定性canonical digest、严格Validate、TTL/currentness与同subject确定性ID；N不等于1、Scope epoch、owner coordinate、摘要或TTL漂移在Tool command前Fail Closed；
- 新增Application-owned `SingleCallToolActionCoordinationFactV1`及线程安全测试Store。状态仅允许`prepared→dispatch_intent→waiting_inspect→completed`；进入`waiting_inspect`必须绑定竞争者唯一`StartClaimID`，相同请求的不同竞争者不能折叠成同一个幂等CAS successor；create/CAS回包丢失按原Scope/ID Inspect，CAS精确`+1`且同ID异内容冲突；
- `SingleCallToolActionCoordinatorV1`严格执行S1→write-ahead→唯一StartClaim CAS→一次同canonical start-or-inspect→S2→Runtime current V4 Inspection→public Association完整Inspect→completed。仅收到自己精确StartClaim CAS回包的调用者可调用Tool；CAS回包丢失、CAS/Inspect恢复失败、竞争失败及预存`waiting_inspect`全部永久只Inspect，NotFound/Unavailable/Indeterminate都不授予Execute；
- Input Reader、Tool、Settlement Reader、Association Reader、commit及return边界均重新取时钟并拒绝回拨；Input `CheckedUnixNano`晚于边界旧时刻、Tool/Settlement/Association调用途中TTL跨越，以及完成CAS后返回前过期均Fail Closed；
- G6A完成结果只包含settled ToolResult中立坐标、current V4 Inspection和Association typed ref。Application不持有Tool Provider Boundary proof，不包含Context Refresh、Continuation、Turn推进、Capability activation或Checkpoint依赖；
- import Conformance与可执行生产包扫描禁止Application依赖Runtime Owner/control/kernel/fakes/foundation及Harness、Tool、Context、Model实现包。

## 实际验证

- `go test -count=100 ./tests -run 'SingleCallToolAction'`：通过；
- `go test -race -count=20 ./tests -run 'SingleCallToolAction'`：通过；
- `go test -count=1 ./...`：通过；
- `go test -race -count=1 ./...`：通过；
- `go vet ./...`：通过；
- `gofmt -l .`：无输出；
- `go test -coverpkg=./... -coverprofile=/tmp/application-g6a-cover.out ./tests`：Application模块总体statement coverage 75.6%；本次G6A核心路径按函数覆盖约60%至100%，Coordinator主入口75.6%；
- Application独占范围的XML/Markdown不涉及新增图；最终`git diff --check`与未跟踪文件空白检查通过后交付。

## 边界

测试只注入Application neutral fixtures，用于证明Coordinator调用顺序、恢复语义、状态机、exact closure和硬停止边界；它不证明Model Projection Reader、Harness聚合Input Adapter、Context CTX-D10 Reader、Tool start-or-inspect/Provider或Runtime Store的跨Owner实现。当前没有生产composition root、持久Backend、RPC、Provider认证或SLA，也未启用G6B Context Refresh/Continuation/Turn推进。
