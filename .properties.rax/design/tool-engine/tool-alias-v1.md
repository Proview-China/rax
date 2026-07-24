# Tool Alias装配期解析 V1

## 1. 裁决

V1只实现Tool Alias，不实现Package Alias。Alias是Tool Registry中的版本化映射：

```text
Owner + Alias name -> exact Tool ObjectRef{ID, Revision, Digest}
```

Alias只允许在Plan/Surface装配阶段、同一个exact Registry Snapshot下解析。解析结果必须把
Alias exact Ref和Tool exact Ref同时写入装配证明，Surface与Run只消费Tool exact Ref；Run、
Action Candidate、Provider Gateway和恢复链禁止再次读取Alias或跟随Alias current。

## 2. Tool Owner对象

`ToolAliasV1`字段固定为：ContractVersion、`ToolAliasRefV1{ID,Revision,Digest}`、
namespaced Alias、Registry Owner、Target Tool ObjectRef、CreatedUnixNano。

- stable ID由`Owner.Domain + Owner.ID + Alias` canonical派生；同名不同Owner物理隔离；
- revision 1 create，successor必须current+1并携expected current full Ref做CAS；
- Digest覆盖完整Alias对象并排除自身；同ID/Revision换内容Conflict；
- Target必须是Registry中active且exact的Tool；Alias不能把inactive/revoked/不同digest Tool复活；
- Alias successor只改变Target/Created与Revision，不改变ID、Owner或Alias name；
- 历史Alias对象保留；current可deprecated/revoked但不能从revoked恢复。

## 3. 唯一Registry与Reader

现有`registry.Registry`是唯一concrete store，同时维护Alias immutable history、current Ref、
current Registry Record和全局Registry Snapshot。不得另建Alias仓或把Alias存在Surface/Plan本地盘
作为唯一事实。

公开Owner-local方法：

```go
SubmitToolAlias(alias, expectedCurrent, now) (registry.Record, error)
InspectToolAlias(exactRef) (ToolAliasV1, registry.Record, bool)
ResolveToolAlias(stableID) (ToolAliasV1, registry.Record, bool)
```

create与successor在同一Registry锁内校验Target active/exact、expected current与全局revision；
lost reply以派生stable ID或exact Ref Inspect，不重复创建不同revision。历史revision重投只能在其
仍为current且内容相同时幂等返回；current已前进后不得回退/ABA。

## 4. SDK装配投影

`RegisterToolAliasV1`只写Tool Registry本地事实，不Connect、不调用Provider、不授予执行权。
`ResolveToolAliasForAssemblyV1`固定执行：

1. fresh context/clock与exact Registry Snapshot S1；
2. 由Owner+Alias派生stable ID并读取current Alias；
3. 验证Alias Record active、revision/digest exact且不晚于Snapshot；
4. exact读取Target Tool与active Tool Record；
5. exact Registry Snapshot S2与clock rollback检查；
6. 返回`ToolAliasResolutionV1{Snapshot, Alias, AliasRecord, Tool, ToolRecord}`深拷贝。

调用方用Resolution.Tool构造`SurfaceSelectionV1`；Resolution.Alias只作装配因果/审计。Alias后续
repoint/revoke产生新Registry Snapshot和新Surface，旧Surface内容与活跃Run不原地改变。

## 5. 非Owner与NO-GO

- Alias不授Authority、Review、Permit、Fence、Capability Grant、Package admission或Provider；
- 不支持Run-time alias、自动latest、semver range、fallback chain、环境变量别名或跨Owner弱读；
- 不实现Package Alias、市场channel、远端Registry同步或production durable backend；
- Package Verify/Admission仍按独立候选和联合门推进，Alias不能绕过它；
- production Assembler/Reconcile接线未闭合前，本切片只属owner-local装配能力。

## 6. 反例

1. create revision非1、successor不是current+1、missing/wrong expected current；
2. same ID/revision换Target、Owner、Alias或Created；
3. Target不存在、inactive、revoked、revision/digest漂移；
4. current已到rev2后重投rev1、ABA、revoked后repoint；
5. exact Registry Snapshot在S1/S2间变化仍返回Resolution；
6. Resolve后Alias repoint导致既有Surface/Run换Tool；
7. Run/Action/Provider代码导入Alias Reader；
8. 64并发same canonical产生多revision，或different Alias被全局串行语义污染。

## 7. 状态

本合同属于业务文档已明确的“Alias仅装配期解析”范围；owner-local Go完成后仍不代表production
Assembler/Reconcile或Package供应链GO。
