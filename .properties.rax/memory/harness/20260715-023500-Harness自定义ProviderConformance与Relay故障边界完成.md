# Harness自定义Provider Conformance与Relay故障边界完成

时间：2026-07-15 02:35（Asia/Shanghai）

Harness已补齐面向未来用户自定义Data Provider的公共Conformance：同一精确请求的并发`Prepare`必须收敛到一个create-once结果，`InspectPrepared`必须读回同一事实；`ExecutePrepared`在验收中只调用一次，随后只允许`InspectLocalAttempt`读取同一结果。报告明确把生产、Binding、Dispatch、Settlement和Completion资格固定为false。

宿主Relay新增取消/超时丢回包恢复，并以黑盒反例确认Permit、payload、attempt、provider binding和delegation任一替换均不能越过宿主边界。TurnState补充64路并发单次线性化、同ref换DomainResult/Pending sidecar/派生Claim/时间冲突，以及裸Session CAS不能跳过Runtime治理引用进入in-flight或从waiting_settlement伪造action/terminal。

Observation权威边界已同步收口：Observation只能把Session推进到`waiting_settlement`，不能自授Settlement、DomainResult或Runtime Outcome；`ApplySettledTurnV2`必须验证Runtime exact Settlement及其schema-bound DomainResult，Action/Input/Completion Claim只能从规范化结果派生。

实际验收：`go test -count=1 ./...`、`go test -count=20 ./...`、`go test -count=1 -race -shuffle=on ./...`和`go vet ./...`全部通过；全模块跨包语句覆盖率79.6%。
