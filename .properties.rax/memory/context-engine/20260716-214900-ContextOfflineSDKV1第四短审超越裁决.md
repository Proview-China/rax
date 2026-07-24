# Context Offline SDK V1第四短审超越裁决

时间：2026-07-16 21:49（Asia/Shanghai）

第四独立短审结论为`NO-GO，P0=5 / P1=5 / P2=0`，本事件超越此前Offline SDK估算。当前Context SDK仍只是`design_candidate/review_pending`，未实现。

状态更新：本事件已由22:06第五短审`NO-GO，P0=2/P1=2`超越；当前有效裁决见[第五短审超越裁决](./20260716-220600-ContextOfflineSDKV1第五短审超越裁决.md)。

超越后的预算：input hard max=24 MiB；Compile-derived generated<=52 MiB、output<=76 MiB；68 MiB generated/100 MiB output只是independent global guards。Wire request/response cap按operation分别是Validate 48/48 MiB、Compile 48/144 MiB、Preview 144/48 MiB、Inspect 144/48 MiB。旧32 MiB input、40 MiB codec和统一request=48 MiB全部清退。

Canonical不再直接DigestJSON含unexported `OfflineContentBundleV1`；候选固定domain/version/discriminator与private canonical body，bundle以sorted ContentRefs+ContentSetDigest closure进入domain digest，wire DTO仅负责唯一base64 chunk编码。Presence递归覆盖SDK顶层及live nested Owner tags，不复制Owner DTO。

Context Owner内部候选为`ContextAwareReferenceStoreV1 / CompileStagedV1 / InspectFrameStagedV1`及exact work limits。Call-scoped workspace固定`Begin/Seal/Export/Abort/Destroy`和`open→sealed→exported|aborted→destroyed`；Abort/Destroy幂等、无ctx，partial Put取消后不可达。Streaming renderer必须golden byte-equal旧renderer，禁止`goroutine+select`假取消，旧Compile/Inspect compatibility wrapper不承诺cancel。

selected-required-rule按live stable sort后每required kind第一个命中candidate冻结，不因其后missing/expiry/authority失败而改选。Required missing=`not_found`+零Response；optional missing=唯一Residual、零Fragment/零token/零Owner Store读。Clone表覆盖全部nested pointer/slice/map/bytes。

验证计划只允许small fixture跑64并发；max-size 24/52/76 MiB与wire边界只跑1/2/4/8并发，记录资源和cancel-to-return，不宣称SLA。本次只更新Context design/plan/memory，不写Go、不stage。
