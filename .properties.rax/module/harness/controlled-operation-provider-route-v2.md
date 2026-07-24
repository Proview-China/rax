# Controlled Operation Provider Route V2模块说明

## 作用

该切面把G6A单Tool动作的Provider接线收敛成一条强类型、可审计的Assembly Route。它只负责声明、装配证明和current读取，不执行Provider、不签发Permit，也不产生Settlement。

## 组成

- `assemblycontract/controlled_operation_provider_route_v2.go`：Harness唯一Declaration事实、封闭Role/Capability、strict canonical与Runtime neutral Ref映射；
- `assemblycontract/controlled_operation_provider_route_conformance_v2.go`：post-binding Conformance、sealed wiring inventory与确定性ConformanceID；
- `assemblycontract/controlled_operation_provider_route_conflict_v2.go`：Harness纯编译结构化Conflict、规范化ProviderTransport/Provider身份与稳定诊断摘要；
- `assemblycompiler/controlled_operation_provider_route_v2.go`：Governance Catalog注册校验、required Manifest extension严格解码、确定性多Declaration merge、从真实PortSpec重建post identity、V1 Route Owner-current Reader S1/S2、sealed alias/version Conflict与全图no-bypass；
- `assemblyadapter/controlled_operation_provider_route_v2.go`及`controlled_operation_provider_route_owner_artifacts_v2.go`：只接exact lookup key的Conformance Builder，package-sealed OwnerSource，verified Compile/ActiveRoute/Wiring独立Reader，Harness-local exact artifact Store、fixture-only线程安全CAS Store、lost-reply恢复、ABA拒绝和Runtime `ControlledOperationProviderRouteCurrentReaderV2` Adapter；
- 对应单元、白盒、并发、故障与import/type Owner测试。

## 不变量

1. 不新增或修改`AssemblyInputV1`字段和摘要算法；Route payload只经required `ComponentManifestV2` extension进入Manifest与Input摘要；
2. Declaration/Conformance同ID immutable create-once；`CurrentID`只调用Runtime公共确定性派生函数，Revision只由Harness Store单调CAS且旧Watermark不得回流；
3. Current Watermark直接闭合ConformanceRef、Generation、Handoff、BindingSet、ActiveRoute和七个独立Binding；WiringInventory由Conformance摘要间接绑定；
4. ProviderTransport与actual Provider是两项独立声明、Candidate、Binding；sealed inventory完整证明ApplicationPort→ToolAdapter→RuntimeGovernancePort→Gateway→Transport→Provider五段链；
5. exact CurrentRef不存在才返回NotFound；同ID任一revision/digest/ref/watermark漂移返回Conflict；
6. lost reply只Inspect原exact Ref；TTL crossing、`now<checked`、S1/S2分别在租约内但`nowS2<nowS1`、任一Binding漂移均Fail Closed；
7. 内存Store只用于确定性测试，不声明生产持久性、Backend或SLA。
8. ProviderTransport/Provider规范化身份完整包含PortSpecDigest与ConflictDomain；post-binding Conflict Side另绑定TransportBinding/ProviderBinding。所有alias路径只产sealed `provider_alias_conflict`并携closed exact AliasSurface；AliasSurface按Kind强制唯一Ref/Module/Port/Capability形状，Dependency精确绑定From/To双端，完整source由AssemblyInputDigest约束。V1/V2双激活只产sealed `active_route_version_conflict`。
9. Conflict按code/phase强制provenance：prebinding只绑定AssemblyInput，postbinding绑定AssemblyInput/Graph/Wiring；Legacy state只允许active/inactive/revoked，后两者必须以调用时current、S1/S2一致的sealed wiring覆盖目标non-active binding，并拒绝同matrix/alias identity下其他active V1。

## 验证基线

本轮门禁基于Harness当前188个`Test`、7个`Fuzz`入口（共195个），按fresh确定性口径`go test -count=1 -coverpkg=./... -coverprofile=<temp> ./...`并以`go tool cover -func=<temp>`读取total，实测跨包语句覆盖率75.2%；最终高重复、Race、Vet、格式、diff、import、零网络与资产校验结果以本任务回传为准。

## 当前边界

Harness Route V2第八独立短审为`YES(P0/P1/P2=0)`；S1/S2跨读取时钟单调性、call-time current exact absence proof与fresh覆盖率口径均已闭合。该YES仅覆盖Route；G6A cross-module fixture、真实Tool Adapter、production composition root、Capability启用、Continuation和Turn推进仍不在本模块产物内。

设计入口见[Route V2设计](../../design/harness/assembly/controlled-operation-provider-route-v2.md)，计划见[Route V2实施计划](../../plan/harness/controlled-operation-provider-route-v2.md)。
