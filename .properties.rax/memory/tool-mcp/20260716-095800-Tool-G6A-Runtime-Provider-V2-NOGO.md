# Tool G6A Runtime Provider V2门禁冻结

时间：2026-07-16 09:58 CST

中央复核确认：Tool本地多采一次Clock、加锁，或在验证Boundary/Enforcement/Handoff后再调用Runtime `ControlledOperationProviderPortV1`，都不能证明actual physical effect入口安全。校验与物理副作用之间仍有调度间隙，V1不得包装升权。

已保留并收口的Tool事实：canonical command完整Action坐标、`Provider.Capability == EffectKind`、Candidate ExpectedOwner/Settlement DomainOwner exact绑定、Store独立拒绝DomainResult Owner漂移，以及Settlement lost-reply只用bounded `context.WithoutCancel`执行exact Inspect。

当前实现裁决：Owner Flow可持久Candidate/Reservation并绑定Runtime Attempt；到`runtime_attempt_bound`后返回unsupported，不CAS新的Provider Boundary、不调用Provider。真实Provider调用数为零，G6A不得宣称YES。

公共Port Delta：Runtime Owner候选`ControlledOperationProviderPortV2/ControlledOperationProviderRequestV2`携带exact Provider Binding、Prepared Attempt及current proof、execute Enforcement/Handoff、Boundary与统一NotAfter。V2 physical executor必须在自身入口采样fresh clock，原子ValidateCurrent后直接产生副作用；若再转调V1仍不闭合。production-neutral V2 seam/fake只可用于Conformance，不代表生产Backend、SLA或能力启用。

后续：等待Runtime V2公共合同、实现与联合Conformance YES；Tool不修改Runtime，不自建兼容Port。
