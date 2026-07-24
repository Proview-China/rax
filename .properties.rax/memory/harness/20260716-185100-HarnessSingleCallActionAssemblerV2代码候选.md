# Harness SingleCall Action Assembler V2代码候选

时间：2026-07-16 18:51（Asia/Shanghai）

## 事件

Application V2 public contract/ports恢复编译并完成canonical补强后，Harness完成Owner-local P3代码候选：`SingleCallToolActionAssemblerV2`同时组装sealed `SingleCallToolActionRequestV2`并实现Application public `SingleCallToolActionInputCurrentReaderV2`。

## 边界

- 构造器只接收`SessionCurrentReaderV4`、`CommittedPendingActionReaderV3`、`SettledTurnDomainResultReaderV3`、Model公开exact Projection Reader、Runtime `AuthorityFactReaderV2`及clock；不接收Session/Fact写口、Tool Port或Governance commit口；
- Request与InputCurrent各自执行Session/Fact/Model/Current/Authority完整S1/S2；nil context、nil receiver、typed-nil依赖、Owner漂移、clock rollback、TTL crossing全部Fail Closed；
- S2 Current V3 expiry不得大于S1；收窄值进入最终Request/InputCurrent proof；`RequestedNotAfter`严格执行负数拒绝、零不增加上界、正数只缩短；
- Identity、ProjectionProof、HarnessOwnerCurrentProof、AuthorityCurrentProof与InputCurrentProjection只在S2完成后的fresh `nowS2`封装；canonical arguments只来自Model exact Reader并逐层deep-copy；
- 不实现Tool Consumer/P4、Application Coordinator、system fixture、Continuation、Capability启用或production composition root。

## 验证

- `go test -count=100 ./applicationadapter ./tests/conformance ./tests/contract`：PASS，14.02s；
- `go test -race -count=20 ./applicationadapter ./tests/conformance ./tests/contract`：PASS，32.71s；
- 中央统一session复核`go test -race -count=20 ./applicationadapter`：PASS，29.663s，exit=0；
- `go test -count=1 ./...`：PASS，1.84s；
- `go test -race -count=1 ./...`：PASS，10.24s；
- `go vet ./...`：PASS，0.26s；
- gofmt、scoped diff-check、trailing whitespace与import boundary扫描：PASS。

## 裁决状态

独立预审已确认主路径`P0=0/P1=0`，三个P2测试补强已经落地并通过上述门；当前状态仍为**代码候选，等待最终独立代码审计**，不自判YES。Application system G6A、P4、G6B与production root继续`NO-GO`。
