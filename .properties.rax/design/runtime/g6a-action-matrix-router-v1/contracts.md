# G6A Action Matrix/Router V1合同与Port边界

## 1. 兼容策略

本切面是OperationScope Evidence V3的候选封闭矩阵扩展，不改现有公共struct、enum、Validate、canonical或digest。联合`YES`后的实现只能：

1. 在Runtime Owner控制的显式matrix catalog中新增唯一Action行；
2. 增加无状态nominal projection helper或等价纯验证规则；
3. 通过现有`OperationScopeEvidenceApplicabilityCurrentReaderV3`注入按Kind路由的Owner Reader；
4. 新增Runtime-neutral、只读、注入式Provider Boundary Ref/Projection/Reader；
5. 增加只依赖公共Port的Conformance与测试。

不得给`OperationScopeEvidenceFactPortV3`增加Applicability Fact Create/Inspect，不得新增Runtime持久化Applicability对象。

## 2. Applicability ref形成规则

输入是Owner Adapter已经验证静态类型的source coordinate。投影必须逐字段相等：

| 公共ref字段 | 唯一来源 | 禁止 |
|---|---|---|
| `Kind` | source.Kind | 改namespace、按维度猜默认值 |
| `ID` | source.ID | 重派生、添加前缀、换ID恢复 |
| `Revision` | source.Revision | 固定为1、单调猜测或使用Reader revision替代 |
| `Digest` | source.Digest | Runtime重seal、用DTO digest或Owner projection digest替代 |

Session与Turn即便四字段文本碰巧相同，也必须因不同静态source type和注册Kind而拒绝type-pun。Action ref必须来自Tool `ActionCandidateV2` exact source；PendingAction ref只能作为Candidate的来源约束，不能填入Action维度。Context ref必须来自ParentFrame source，不得使用下一Turn Frame或Application `ParentFrameCoordinate`直接冒充。

## 3. Reader路由与current投影

Reader路由键至少包含`dimension + exact namespaced Kind + Owner contract version`。每个路由只允许一个Owner Adapter；重复、缺失、未知Kind或Reader unavailable都Fail Closed。Reader返回现有公共`OperationScopeEvidenceApplicabilityCurrentProjectionV3`，Gateway必须验证：

- `projection.Fact == request.Fact`逐字段exact；
- `ExecutionScopeDigest`与Operation Scope exact；
- `Current=true`；
- Checked/Expires有界且调用时未过期；
- projection digest重算一致；
- Owner特有subject关系闭合。

Owner特有检查：

| 维度 | 必须复读 |
|---|---|
| Run | Tenant/Scope/Run stable identity、允许执行状态、revision/digest/current TTL |
| Session | Harness Session、phase=`waiting_action`、PendingAction、S1/S2、短租约 |
| Turn | exact Turn、Session revision/digest、PendingAction、S1/S2、短租约 |
| Action | `ActionCandidateV2`、Reservation来源、PendingAction、Run/Session/Turn/Effect/Owner |
| Context | ParentFrame、Manifest、Generation current、Run/Session/Turn、Recipe/Authority上界 |

Runtime Router不复制这些领域检查；它只验证公共投影并把调用路由给唯一Owner Reader。

## 4. Policy与Generation

唯一Policy profile为`praxis.tool/single-call-action-v1`。Evidence Policy与Applicability Policy都必须：

- 精确绑定`run + praxis.tool/execute`；
- 要求Generation；
- 把Run/Session/Turn/Action/Context全部设为`required`；
- 绑定当前Generation及Generation-Binding Association；
- TTL不超过全部Owner current projection、Generation/Binding、治理事实与phase Enforcement上界的最小值。

字段缺省不表示optional；缺省、unknown或TTL不可证明一律拒绝。

## 5. 两phase关系

prepare与execute必须使用不同Qualification ID、Handoff ID、Consumption ID、Event ID/source sequence及对应4.1 phase ref。execute请求必须引用其exact prepare链，但不得复用prepare Evidence资格。phase交换、重复、缺失或额外phase都零写拒绝。

Provider boundary顺序不可改写：

```text
exact/current execute Enforcement 4.1
-> exact/current execute Evidence Handoff (same Attempt + execute phase)
-> Tool Owner Watermark CAS provider_boundary_crossed
   (monotonic binds both public exact refs)
-> public current boundary projection reread
-> at most one Provider call
-> Provider response/Observation
-> corresponding Evidence Consumption
```

Runtime不写Tool Watermark，也不把它变成Runtime Fact。受控test Provider seam必须要求调用方提供exact Boundary Ref，并在调用前通过注入Reader逐字段复读；缺失、漂移、跨Attempt、非current或Reader unavailable时Provider调用计数为0。boundary CAS成功即视为Provider可能已调用，lost reply/崩溃只能Inspect原Attempt/Observation。prepare与execute Consumption都不得预填到boundary。

## 6. Runtime-neutral Provider Boundary只读合同

候选公共类型全部放在未来`ports/operation_scope_evidence_action_v3.go`，但不修改任何既有V3/V4/4.1类型或digest：

- Boundary只读合同版本固定为SemVer `1.0.0`；
- Ref与Projection使用不同canonical domain：`praxis.runtime.operation-provider-boundary-ref/v1`与`praxis.runtime.operation-provider-boundary-current-projection/v1`；
- Ref的ID/Revision/Digest逐字段来自Tool Owner Watermark exact坐标，Runtime不得重新派生或重seal；Projection digest只封存本次current读取，不替换Ref digest。

```text
OperationProviderBoundaryRefV1 {
  ID, Revision, Digest
}

OperationProviderBoundaryCurrentProjectionV1 {
  ContractVersion
  Ref
  Operation                 OperationSubjectV3
  OperationDigest
  OperationScopeDigest
  Attempt                   OperationDispatchAttemptRefV3
  ExecuteEnforcement        OperationDispatchEnforcementPhaseRefV4
  ExecuteEvidenceHandoff    OperationScopeEvidenceProviderHandoffRefV3
  Stage                     provider_boundary_crossed
  CheckedUnixNano
  ExpiresUnixNano
  Digest
}

OperationProviderBoundaryCurrentReaderV1 {
  InspectCurrentOperationProviderBoundaryV1(ctx, exactRef)
    -> exact current projection
}
```

严格规则：

1. `Ref`至少包含非空稳定ID、非零Revision和有效Digest；它是Tool Watermark exact历史坐标的Runtime-neutral nominal ref，不授执行权；
2. Projection必须逐字段exact绑定输入Ref、sealed `OperationSubjectV3`及其digest、完整Operation Scope digest、Runtime Attempt、execute Enforcement 4.1 ref和execute Evidence Handoff ref；
3. `Stage`是封闭单值`provider_boundary_crossed`；其他字符串、空值或未来stage不能由V1 Reader返回current；
4. `CheckedUnixNano < ExpiresUnixNano`，Provider seam在调用前使用fresh clock验证尚未到边界；Projection digest覆盖除自身外全部字段并重算一致；
5. execute Enforcement与Handoff必须各自Validate、phase=`execute`、绑定同一Operation/Attempt，且与Provider seam此前current复读得到的public refs逐字段相等；
6. Reader只读且注入式。Runtime不import Tool，不创建Boundary Fact/Store，不为Tool Watermark生成ID/revision/digest；Tool Owner Adapter把自身Watermark exact坐标无损投影为Ref，并在每次Inspect中复读Watermark current；
7. Reader unavailable/NotFound、Ref或Projection type-pun、Operation/Scope/Attempt/ref漂移、TTL边界或clock rollback全部Fail Closed且Provider计数为0。

受控test Provider seam的唯一允许顺序为：Validate Boundary Ref → Reader exact current Inspect → 与execute Enforcement/Handoff/Attempt逐字段交叉比较 → fresh TTL检查 → 至多一次Provider调用。Boundary Projection不是Permit、Enforcement、Evidence资格、Handoff、Consumption或DomainResult。

## 7. 候选实现落点

联合`YES`后才允许评审以下最小Runtime独占Delta：

| 候选路径 | 内容 |
|---|---|
| `ExecutionRuntime/runtime/ports/operation_scope_evidence_action_v3.go` | matrix常量、Applicability nominal projection，以及`OperationProviderBoundaryRefV1`/current projection/Reader；只读且不含Fact Store |
| `ExecutionRuntime/runtime/kernel/operation_scope_evidence_action_router_v3.go` | 封闭Kind路由、current投影校验与Gateway装配 |
| `ExecutionRuntime/runtime/conformance/operation_scope_evidence_action_v3.go` | public-only matrix/router/Boundary Reader Conformance；验证零Provider负例，不获得Tool写口 |
| `ExecutionRuntime/runtime/tests/**/operation_scope_evidence_action_v3_test.go` | unit、whitebox、blackbox、fault与race负例 |

Harness、Tool、Context、Application与Model Adapter不在Runtime独占路径内，须由各Owner在其资产`YES`后分别实现。若Model Projection Exact Reader仍未`YES`，整个G6A实现保持NO-GO。
