# Agent Host Journal 与 Port Panic 终审返修

- 时间：2026-07-18 00:32 CST
- 范围：`ExecutionRuntime/agent-host` H1-H2；H3-H5 仍未实现。

## 返修结果

- Journal successor 禁止 BindingAttempt 首次直写 bound/unknown，禁止 ConstructionAttempt 首次直写 constructed/unknown，并限制每个 successor 只能尾部追加一个 planned attempt。
- Factory 返回有效 handle/ref 后先纳入确定 cleanup 集，再提交 constructed progress；CAS 可能已提交但首轮 Inspect 失败时，Host 二次 exact recovery 后执行 cleanup 并进入 Indeterminate。
- unknown progress 持久失败不再忽略，也不声称 Unknown 已持久；错误包含 `unknown_progress_not_persisted`。
- Journal Create/CAS port panic 按 lost/unknown reply 立即 Inspect exact desired；Inspect 再 panic 时维持 UnknownOutcome。
- Ready Inspect 的 fresh clock 必须不低于 Journal Updated watermark。
- Decoder、Assembler、Compiler、Binding、Readiness、Journal、Clock panic 全部隔离为 typed closed error；Binding start panic 仍沿原 Attempt Inspect。
- Plan 已将 H1-H2 标为 owner-local 完成；H3-H5 保持未完成。

## 验证门

- targeted ordinary 100；
- targeted race 20；
- full ordinary/race/vet；
- gofmt、import boundary、diff check。
