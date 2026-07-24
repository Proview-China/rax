# Runtime G6A Action Matrix/Router V1设计

## 1. 状态与目标

- 当前状态：中央交叉实现性复核发现Boundary Reader公共缺口，上一轮独立Review不作为最终裁决；本修订等待重新联合评审，联合`YES`前不写Runtime、Harness、Application、Tool、Context或Model代码；
- Runtime实现保持冻结；本资产不修改OperationScope Evidence V3、Dispatch V4.0、Enforcement 4.1或Settlement V4既有公共字段、canonical与digest；
- 目标：为单Call Tool Action登记唯一封闭的Run内Evidence V3矩阵，并冻结五维Owner-current Reader路由、nominal projection和Fail Closed边界。

本设计不是万能Hook、批处理、Checkpoint或生产composition root。它只允许G6A测试fixture在全部公共Reader就绪后验证N=1 Action纵向链。

## 2. 唯一封闭矩阵

| 字段 | 唯一允许值 |
|---|---|
| `OperationScopeKind` | `run` |
| `EffectKind` | `praxis.tool/execute` |
| `PolicyProfile` | `praxis.tool/single-call-action-v1` |
| Generation | `required` |
| Run | `required` |
| Session | `required` |
| Turn | `required` |
| Action | `required` |
| Context | `required` |

除这一行外，所有Operation kind、Effect kind、Policy profile、缺Generation或任一维度非`required`的组合都unsupported，并在Evidence Issue、Tool写入和Provider接触前零写拒绝。不得用`custom`、activation profile、空Context或可选Action扩表。

## 3. Owner与current来源

| 维度/前置 | exact source | current Reader Owner | Runtime允许做什么 |
|---|---|---|---|
| Run | Runtime真实Run current Fact | Runtime Run Owner | 复读同Tenant/Scope/Run identity、状态、revision/digest与TTL；不从Session推导Run |
| Session | Application Request中的Harness distinct Session source coordinate | Harness Owner；`runtimeadapter`实现既有`OperationScopeEvidenceApplicabilityCurrentReaderV3` | 只把`Kind/ID/Revision/Digest`无损投影为公共ref并按Kind调用Reader |
| Turn | Application Request中的Harness distinct Turn source coordinate | Harness Owner；同上 | 与Session使用不同nominal type、canonical domain和Kind路由，禁止互换 |
| Action | Tool Owner `ActionCandidateV2` exact source | Tool ActionCandidate current reader | 绑定Candidate本身及Run/Session/Turn/PendingAction来源；不得指向PendingAction或Application DTO |
| Context | Context Owner ParentFrame exact source | Context ParentFrame current reader | 复读产生本次Tool Call的ParentFrame/Generation，不创建下一Turn Frame |
| Generation | exact Generation与Generation-Binding current association | Runtime Generation/Binding current readers | 作为Evidence Policy强制前置；不可缺省或从Context ID猜测 |
| Model Observation | 完整Tool Call Projection exact ref | Model Owner Projection Exact Reader | 只作为形成N=1 Application Request的前置；该Reader资产尚待Model Owner联合`YES` |

每个Owner source的精确namespaced Kind必须由对应Owner公共合同冻结并注册到封闭路由表；Router没有默认分支、字符串前缀匹配或动态自注册。Reader缺失、Kind未注册、source revision/digest漂移、返回Fact不等于输入、scope不符或TTL跨界全部Fail Closed。

## 4. nominal projection与Fact Owner边界

live `OperationScopeEvidenceFactPortV3`没有Applicability Fact的Create/Inspect。G6A不得新增Runtime Applicability Fact Store、Fact Reader、缓存或第二Owner。

Runtime nominal projection只执行：

```text
Owner source coordinate {Kind, ID, Revision, Digest}
  -> OperationScopeEvidenceApplicabilityFactRefV3
     {同一个Kind, 同一个ID, 同一个Revision, 同一个Digest}
```

它不重新seal、不生成ID/digest、不延长TTL、不读取Owner Store，也不把公共ref变成Evidence资格。Application中立Session/Turn source coordinate保留各自静态类型；宿主Adapter解包为四个标量后调用Runtime纯投影规则，Runtime不import Application或Harness类型。

Evidence Gateway只有在矩阵、Policy、Generation、五维required refs以及注入的`OperationScopeEvidenceApplicabilityCurrentReaderV3`全部验证current后才可Issue Qualification。公共ref、Application DTO、PendingAction和Owner Observation都不能跳过该门。

## 5. phase与Provider边界

prepare和execute必须分别拥有独立的：

- Evidence V3 Qualification；
- Provider Handoff；
- Consumption；
- Event/source sequence；
- Enforcement 4.1 phase ref。

两phase不得交换、复用或用一次消费覆盖两次。prepare/execute Consumption都只能在对应Provider响应或Observation已经形成后写入DomainResult因果链，不得在Provider boundary前预填或推测。

真实Provider调用前的顺序固定为：

1. Runtime公开Reader证明exact/current execute Enforcement 4.1；
2. Evidence Owner公开Reader证明exact/current execute Evidence Handoff，且绑定同一Attempt与execute phase；
3. Tool Owner CAS同一`SingleCallToolActionCoordinationWatermarkV1`到`provider_boundary_crossed`，单调绑定上述两个public exact refs；
4. 受控Provider seam以exact `OperationProviderBoundaryRefV1`调用注入式`OperationProviderBoundaryCurrentReaderV1`，复读Tool Owner已提交的`OperationProviderBoundaryCurrentProjectionV1`；
5. CAS成功后才允许至多一次Provider调用。此刻起即视为可能已调用；lost reply或崩溃只Inspect原Attempt/Observation；
6. Provider响应/Observation形成后，才分别完成对应Consumption并继续DomainResult。

Runtime不创建、不CAS也不拥有Tool Watermark或Boundary Fact/Store。Runtime-neutral Boundary Ref/Projection只提供跨Owner只读坐标和current证明；Tool Owner Adapter实现Reader并复读自己的Watermark。G6A Runtime只冻结Provider seam必须消费该current projection；缺失、漂移、跨Attempt、非current或无法复读时零Provider。prepare handoff、Begin、Permit、Evidence Qualification和Provider Observation本身都不是execute授权。

Provider Observation不是Tool `DomainResultFact`。权威收口顺序保持：

```text
Tool Owner ActionCandidate/Reservation
-> Runtime Dispatch V4 + Enforcement 4.1
-> Evidence V3 prepare/execute独立Qualification/Handoff
-> Tool Owner provider_boundary_crossed current proof
-> Provider response/Observation
-> Evidence V3 prepare/execute独立Consumption
-> Tool Owner DomainResultFact
-> Runtime Operation Settlement V4
-> Tool Owner ApplySettlement
-> settled ToolResultV2
-> Runtime current V4 Inspection
-> public Association Inspect
```

## 6. G6A硬停止线

G6A唯一输出是：

1. settled `ToolResultV2`；
2. current `OperationInspectionSettlementRefV4`；
3. 通过Runtime公开只读治理面Inspect得到的完整Association。

到此必须停止。G6A不启用Capability，不调用Context Refresh，不构造Continuation，不推进Harness Turn，也不进入Checkpoint。`N>1`和万能Hook继续unsupported。

## 7. 依赖与接线门

进入代码前必须由联合评审确认以下只读资产：

- Application中立Session/Turn source coordinates与窄Port；
- Harness `CommittedPendingActionReader`及`runtimeadapter` current Reader；
- Tool `ActionCandidateV2` current reader与settled `ToolResultV2`；
- Runtime-neutral `OperationProviderBoundaryRefV1`、`OperationProviderBoundaryCurrentProjectionV1`和注入式`OperationProviderBoundaryCurrentReaderV1`；Tool Owner Adapter负责实现并复读自身Watermark；
- Context ParentFrame current reader；
- Model Projection Exact Reader（当前仍待Model Owner资产`YES`）；
- Runtime Run、Generation/Binding、Evidence V3、Dispatch V4.0、Enforcement 4.1及Settlement V4现有公共合同。

缺任一Reader、Kind路由或current projection即NO-GO。当前没有production composition root；G6A只能由显式test fixture注入Readers与Adapters。生产root仍是G6B前宿主Owner残余，不能由Runtime、Harness Assembly或Application测试fixture冒充。

## 8. 不做事项

- 不设计Applicability Fact Store、万能Reader注册表或动态Hook；
- 不把Action维度指向PendingAction、Model Observation或Application DTO；
- 不把Provider Observation解释成DomainResult或Settlement；
- 不设计`N>1`、Checkpoint、Context Refresh、Continuation或Turn推进；
- 不修改activation first slice或把Action矩阵隐式加入live Catalog；
- 不声明生产Backend、composition root、availability或SLA。

## 9. 资产

- [合同与候选Port Delta](./contracts.md)
- [测试矩阵与负例](./test-matrix.md)
- [原始流程图](./g6a-action-matrix-router-v1.drawio)
- [实施计划](../../../plan/runtime/g6a-action-matrix-router-v1.md)
