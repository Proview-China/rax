# AgentRunService 横向公共合同 v1

状态：owner-local 公共合同已实现（2026-07-30）；production NO-GO

## 1. 结论

本切片只冻结两组 Go 公共合同：CrossLanguageWirePrimitivesV1 与 AgentRunServiceV1 skeleton。

前者定义跨语言安全的 version/capability negotiation、exact ref、十进制数字和 UnixNano；后者定义 Inspect、Watch、Cancel、Stop 的 transport-neutral request/result、command envelope、receipt 与 ports。

这只是 console-ready wire foundation。Console 集成尚未开始；无页面 VM、无 Console 命令、无 TypeScript DTO。codegen 与 TS backend 必须在后续独立 PR 处理。

## 2. Owner 边界

| 能力 | 唯一权威 | AgentRunService 允许做什么 | 禁止推断 |
| --- | --- | --- | --- |
| Inspect | Host、Runtime Run、Application command 各自 Owner | 按 exact target 读取并返回 owner projection | Host/Harness 观察不能改写 Runtime outcome |
| Watch | owner event/current reader | 返回有界、连续的 owner event page | 不创建第二事件日志，不伪造 replay |
| Cancel | Runtime ApplicationCommandCancelRunV2 入口 | 提交精确 command envelope，返回 command receipt | accepted/completed receipt 不等于 Run cancelled |
| Stop | HostV3 lifecycle | 绑定 exact Host current 后提交 Stop | Stop 不等于 Cancel，不声称 Run outcome |

UnknownOutcome 与 Indeterminate 必须作为结构化 disposition/fault 返回，不能被 transport 层压成无语义的 500。丢失 command reply 后，只能 Inspect 原 command；不得创建第二 command ID 或替换 idempotency key。

## 3. 跨语言 wire primitive

JavaScript 无法无损承载任意 uint64。以下字段在 JSON wire 上必须是规范十进制字符串：

- revision、epoch、sequence、watermark、afterSequence；
- submittedAtUnixNano、notAfterUnixNano、checkedUnixNano、expiresUnixNano；
- 其他来自 Go uint64 或 int64 纳秒坐标的数字。

uint64 使用无符号规范十进制字符串；通用 int64/UnixNano 使用 canonical signed int64 decimal，支持完整 int64 范围并拒绝 +、指数、前导零与 -0。具体业务时间字段再要求正数。time.Time 使用独立 canonical UTC RFC3339Nano Z 字符串，不能与 UnixNano 互换。公共 wire DTO 不直接嵌入含 JSON number 的 owner struct。

ExactRefWireV1 必须完整绑定 kind、id、revision、digest。任何字段缺失或漂移都 fail closed；不得仅按可变 ID discovery 后再包装为 exact ref。

## 4. Version 与 capability negotiation

客户端发送按偏好排序、无重复的 supported contract versions，以及无重复的 required/optional capabilities。服务端只可选择双方共同 version，并返回排序、无重复的 granted capabilities。

required capability 缺失时返回结构化 CAPABILITY_UNAVAILABLE disposition，不可静默降级。v1 capability 仅冻结：agent_run.inspect、agent_run.watch、agent_run.cancel、agent_host.stop、command.inspect_original。

## 5. Command envelope 与 receipt

Cancel 与 Stop 共用 AgentRunCommandEnvelopeV1：

- commandId、kind、target exact refs；
- actor、authorityRef；
- idempotencyKey、canonicalPayloadDigest、requestDigest；
- expectedCurrent 的 exact ref 与 epoch；
- submittedAtUnixNano 与 notAfterUnixNano。

相同 idempotency key 与相同 canonical payload 必须返回原 receipt；相同 key 与不同 payload 必须 conflict。commandId 相同但 requestDigest 或 expected current 漂移也必须 conflict。

receipt 只陈述 command 状态与 owner receipt exact ref。它不得把 command accepted/executing/completed 翻译成 Runtime Run outcome。

CommandResult 必须把 requestDigest、TraceID、receipt 与 validity window 一起封口；checked 必须位于原 command 的 submittedAt/notAfter 区间内，expires 不得超过 notAfter，receipt recordedAt 不得晚于 checked。

## 6. Inspect 与 lost reply

AgentRun Inspect 绑定 HostID、StartID、HostStartClaim、Runtime execution scope current、Run current 与 authority epoch。结果只承载 exact owner projections；聚合 disposition 只能是 OBSERVED、NOT_FOUND 或 INDETERMINATE，并与原 request TraceID 精确绑定。

InspectOriginal request 必须携带原 command exact ref + idempotencyKey + requestDigest，或单独携带原 attempt exact ref；两者不能同时出现。lost reply 恢复只允许检查这个原始对象。若 Owner 仍无法证明结果，返回 INDETERMINATE/UNKNOWN_OUTCOME，并保留 INSPECT 指令。

## 7. Event stream

Watch 使用 owner event stream、retention current 的 exact ref、cursor 与 afterSequence。每个 event 同时绑定 source owner exact ref 与 subject exact ref，携带 payloadVersion/payloadDigest、occurredAt 与 observedAt；canonical event digest 同时写入 EventRef.Digest 与 EventDigest，拒绝 splice：

- event sequence 必须从 afterSequence + 1 连续递增；
- next cursor 必须绑定同一 stream exact ref，并等于最后一个事件 sequence；
- timeout 可以不推进 sequence；
- cursor 回退、stream/retention current 漂移、sequence 缺口或无法证明连续性时，只返回 RESYNC_REQUIRED；
- RESYNC_REQUIRED 不携带伪连续 events/cursor，fault.CurrentRef 必须绑定 exact retention current，并要求重新 Inspect。

多 Owner event retention、全序 replay 与 gap repair 没有在本切片冻结。

## 8. Stop 与 Cancel 分离

Cancel 映射 Runtime ApplicationCommandCancelRunV2；Stop 映射 HostV3 Stop。二者 command kind、target current 与 owner receipt 分离，不允许自动互转。Stop cleanup closed 也不能替代 Runtime termination report。

## 9. 明确未冻结

- Build、Run、Create、Start public API；
- Builder/AgentPackage coordinates 与 executable assembly；
- production composition root 与 deployment wiring；
- Console 页面、页面 VM、Console 命令、认证与 transport binding；
- TypeScript DTO、codegen、TS backend；
- 多 Owner replay、retention 与 gap-repair 协议。

当前 production NO-GO。

## 10. Transport admission 与 fault

StrictJSONDecoderV1 在调用 service 前拒绝超限 payload、任意层 duplicate key、unknown field 与 trailing document。固定 enum（包括冻结的 v1 capability）遇到未知值必须 fail closed，不静默映射；Event kind 是含 namespace/local-name 的 open identifier，不冒充固定 enum。

公共 fault 保留 INVALID_ARGUMENT、UNAUTHENTICATED、FORBIDDEN、NOT_FOUND、REVISION_CONFLICT、IDEMPOTENCY_CONFLICT、PRECONDITION_FAILED、CAPABILITY_UNAVAILABLE、UNAVAILABLE、RATE_LIMITED、UNKNOWN_OUTCOME、INDETERMINATE、INTERNAL，以及 reason、command/attempt/current exact ref、TraceID、RETRY/INSPECT 指令。
