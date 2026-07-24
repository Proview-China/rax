# Runtime Evidence Subject Current V1首审NO返修

- 时间：2026-07-16 21:58:20 +08:00
- 状态：独立资产审查NO（P0=2/P1=2）已进入Runtime asset-only返修；未授权Go实现。

本次返修冻结：ProjectionID只由固定domain/version与SubjectKeyDigest派生，同subject只允许同ID revision+1；完整Projection canonical仅清Ref.Digest与ProjectionDigest，覆盖所有其他字段并把同一digest写回两者。request/projection新增expected Registration、Reader Binding与Capability exact refs，Gateway未来必须在S1/S2逐字段复读回扣。

canonical domain/version/object discriminator、readability/presence闭表、Owner mutation stable key及lost-reply/no-ABA反例已从建议升级为硬合同。当前仍是Go NO-GO，不修改Continuity，不选择production backend/root/durability/SLA，不stage、不commit。
