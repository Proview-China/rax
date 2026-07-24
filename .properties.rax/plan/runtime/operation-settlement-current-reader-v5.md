# Operation Settlement Current Reader V5实施计划候选

状态：**第二次独立代码短审YES（P0/P1/P2=0/0/0），本纵切完成**。

设计入口：[README](../../design/runtime/operation-settlement-current-reader-v5/README.md)、[Port Delta](../../design/runtime/operation-settlement-current-reader-v5/port-delta.md)、[测试矩阵](../../design/runtime/operation-settlement-current-reader-v5/test-matrix.md)。

## P1：联合冻结

- [x] 盘点live V5 current request、inspection、Governance、Gateway、Store和Conformance；
- [x] 确认只需抽取既有current Inspect，不新增对象、digest或实现路径；
- [x] 确认V3同型Reader仅为兼容证据，不自动授权V5代码；
- [x] 识别Gateway当前缺少request Operation/Effect与returned Bundle exact交叉的P0；
- [x] 冻结Gateway顺序为request Validate→dependency preflight→Fact Inspect→Inspection Validate→Operation/Effect exact→return；
- [x] 冻结Conflict必须返回零Inspection，不能泄露错误backend的closure；
- [x] 冻结Reader 1方法、Governance 6方法、Fact Port不变及V5对象/digest不变；
- [x] Runtime Owner资产自审P0/P1/P2=0/0/0；
- [x] Runtime/Sandbox联合Review精确签名、能力收窄和错误语义；
- [x] 用户明确授权最小Go Delta。

## P2：获授权后的最小实现

- [x] 在`runtime/ports/operation_checkpoint_settlement_v5.go`新增`OperationSettlementCurrentReaderV5`；
- [x] Governance兼容嵌入Reader并删除接口内重复current方法声明，保持最终方法集不变；
- [x] Kernel Gateway在返回前exact校验request Operation/Effect与Inspection Bundle，错误backend回值Conflict且不泄露；
- [x] 新增compile、method-set、capability narrowing、typed-nil及import-boundary测试；
- [x] 新增malicious backend返回另一Operation/Effect结构有效Inspection的零泄露反例；
- [x] public Conformance仅持Reader验证current closure，不取得Settle或Fact Port；
- [x] 证明Store无需适配，Gateway只增加上述读取边界校验。
- [x] 首轮独立代码短审：识别raw Fact Port误装配、验证顺序与恶意backend反例不足；
- [x] 新增Gateway-backed provider marker与Kernel facade constructor，public Conformance只接收该provider；
- [x] 补malformed-before-drift、同ID跨Tenant/Scope/nested ref、Unavailable/Indeterminate透传、DeepEqual零Inspection与零副作用反例；

## P3：验证与收口

- [x] targeted ordinary `count=100`；
- [x] targeted race `count=20`；
- [x] Runtime full ordinary/race/vet；
- [x] gofmt、diff-check；
- [x] 第二次独立代码短审YES，module/memory状态升级为最终YES。

## 非目标

不修改Sandbox/Harness/Tool/Model/Application，不接production backend/root，不实现Restore，不改变V5 Settlement对象、shared guard或Owner语义。

本Plan全部已勾选项对应live实现、实际门禁和第二次独立代码短审YES；不授production backend/root/durability/SLA。
