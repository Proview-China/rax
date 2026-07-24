# Checkpoint ManifestSeal V2.1 Exact Reader Delta

时间：2026-07-17 23:32 CST

用户授权最小Runtime Delta后，Runtime public `CheckpointManifestSealContractVersionV2`升为`2.1.0`并完成reference实现：

- 完整Continuity OwnerBinding、Tenant/Scope、Seal ID/revision/raw digest exact lookup；
- expected Runtime Participant Set digest与sorted current Participant closures Reader request；
- Runtime typed Participant closure到Continuity exact ref的唯一结构化映射；
- external SHA-256 canonical normalization；
- Gateway在Consistency CAS前执行Participant/Seal S1/S2，并拒绝scope、Owner、closure或digest漂移；
- Continuity只读Adapter逐项校验Manifest/Attempt/Barrier/EffectCut/frozen set/Participant/Context/Artifact binding。

验证：Runtime与Continuity targeted ordinary100、race20、full ordinary、full race、vet均PASS；覆盖完整Owner drift、delimiter-safe identity、invalid digest、S1/S2 drift、typed-nil与lost Inspect reply。

边界：该reference Reader不创建Runtime或Continuity Fact，不调用Participant/Provider，不实现Checkpoint capture或Restore，不解锁Harness/Application/Participant production root。

