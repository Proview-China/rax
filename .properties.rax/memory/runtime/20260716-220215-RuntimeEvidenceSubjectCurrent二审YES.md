# Runtime Evidence Subject Current V1二审YES

- 时间：2026-07-16 22:02:15 +08:00
- 状态：asset-only第二次独立只读复审YES，P0/P1/P2=0/0/0；未授权Go实现。

首审P0/P1返修已冻结：ProjectionID只由固定domain/version与SubjectKeyDigest派生，同subject使用同ID revision+1；完整Projection canonical只清Ref.Digest与ProjectionDigest并把同一digest写回。request/projection exact绑定Registration、Reader Binding和Capability，Gateway未来在S1/S2逐字段复读回扣。

domain/version/object discriminator、readability/presence闭表、Owner mutation stable key、lost-reply progressed recovery、same-ref换body、watermark换ID、digest回流及registration/binding drift矩阵均已闭合。当前不写Go，不修改Continuity，不选择production backend/root/durability/SLA，不stage、不commit。
