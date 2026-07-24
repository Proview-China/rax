# Memory Retrieval Domain Gateway V1 Port Delta

状态：**公共治理Delta，等待Memory/Application/Runtime/Review/Evidence联合Review；当前所有远程路径unsupported且Provider调用数必须为0**。

## 1. 用例与Owner

该Delta仅承载`praxis.memory/remote-query`与`praxis.memory/remote-content-read`。Memory Owner拥有查询语义、Retrieval Observation/Result、DomainResultFact、Inspect/CAS与ApplySettlement；Application拥有跨域编排；Runtime拥有Operation、Attempt、Permit、Begin、Fence和Settlement线性化；Review/Evidence Owner分别拥有Verdict与Ledger。Provider只返回Observation。

Gateway不得写Runtime Outcome/Binding/Policy/Trust，也不得让Context/Harness直接调用Provider。

当前G6A closed applicability/evidence/settlement matrix只证明Tool类动作，不证明Memory Retrieval适用。Checkpoint V5只服务Checkpoint语义，也不得复用、别名或包装成Retrieval合同。本Delta必须等待对应Owner联合冻结三个**retrieval-specific additive version**：Retrieval Applicability、Retrieval Evidence、Retrieval Settlement；具体版本号、schema与兼容映射当前均未冻结，不得自行命名为既有版本或宣称已支持。本地`MemoryContextSourceCurrentReaderV1`不依赖这三个远程版本，继续保持纯本地只读面。

## 2. 拟议Port与治理坐标

合同名：`praxis.memory/retrieval-domain-gateway/v1`。

```text
MemoryRetrievalDomainGatewayV1
  Retrieve(MemoryGovernedRetrievalRequestV1)
    -> MemoryRetrievalDispatchReceiptV1

  ReadRemoteContentExact(MemoryGovernedRemoteContentRequestV1)
    -> MemoryRemoteContentDispatchReceiptV1

  InspectOriginalAttempt(MemoryRetrievalEffectAttemptCoordinateV1)
    -> MemoryRetrievalEffectInspectionV1

  ApplySettlement(MemoryRetrievalApplySettlementRequestV1)
    -> MemoryRetrievalApplySettlementResultV1
```

每个`Retrieve`与`ReadRemoteContentExact`请求必须携带并canonical绑定：

- OperationSubject、Operation ID/revision/digest、Domain Intent与Reservation exact refs；
- Admission、Review Verdict、Permit及Permit expiry；
- Begin、Delegation/Prepare、Prepare Enforcement、Execute Enforcement exact refs；
- 原Effect Attempt ID/revision/digest、Idempotency Key、Conflict Domain；
- Identity/Fence epoch、Authority、Policy、Scope、Purpose、Sensitivity、预算与NotAfter；
- Provider/Remote Target ref、请求Payload ref/digest；
- Query/View/Expected Watermark；远程正文另含Record/Content expected exact refs。

缺失任一治理坐标、坐标摘要不匹配、Permit过期、Fence漂移或Enforcement未通过时，Gateway必须在Provider执行前拒绝。

## 3. 固定执行顺序与输出

```text
Domain Intent/Reservation
 -> Admission
 -> Runtime Permit
 -> Runtime Begin
 -> Delegation/Prepare
 -> prepare Enforcement
 -> execute-point Enforcement
 -> Provider Execute或Inspect
 -> Provider Observation
 -> Memory Owner Inspect并持久化Retrieval Observation/Result
 -> Memory DomainResultFact CAS
 -> retrieval-specific Evidence append（待新加法版本）
 -> retrieval-specific Runtime Operation Settlement(ref only，待新加法版本)
 -> Memory ApplySettlement CAS
 -> Current Reader复读
```

Dispatch Receipt只证明请求被接受或派发，不证明Provider成功。Provider响应只形成`ProviderObservationRef`。正式`MemoryRetrievalObservationV1/ResultV1`必须由Memory Owner重算canonical、执行领域Inspect并CAS。

`DomainResultFact`必须精确绑定Operation、原Attempt、Provider Observation、Observation/Result exact refs、CAS before/after、Evidence requirement、Residual和canonical digest。`ApplySettlement`必须精确绑定同一DomainResultAssociation与Runtime Settlement exact ref；错Operation、错Attempt、错DomainResult revision/digest或错Settlement均拒绝。

## 4. Unknown、Evidence与支持门禁

Begin后任何丢回包都只能调用`InspectOriginalAttempt`，其输入必须复用原Operation/Attempt/Permit/Prepare/Enforcement/Provider坐标；不得重发Retrieve、换Provider或换Payload。Inspection本身需要远程Effect时，必须采用Runtime批准的非递归Inspection Operation，并关联原Attempt。

以下公共能力没有联合Review YES前，`remote-query`与`remote-content-read`保持`unsupported`且Provider/Resolver调用数为0：

1. Operation/Attempt/Permit与双Enforcement坐标可由Application可靠传递；
2. Provider Observation可持久化并由Memory Owner独立Inspect；
3. Evidence Owner冻结retrieval-specific additive Evidence applicability/schema/version，可引用精确DomainResultFact；
4. Runtime/Application Owner冻结retrieval-specific additive Applicability与Settlement schema/version，使Settlement能ref-only关联该DomainResult；
5. Memory ApplySettlement能验证精确Association并闭表。

未完成Evidence、Runtime Settlement和ApplySettlement闭表的远程结果不得交给Context Current Reader，也不得伪装成本地成功、Partial Coverage或空命中。

现有Tool G6A closed matrix、通用`OperationScope-aware Evidence`文字或Checkpoint V5都不能单独解除该门禁；三项retrieval-specific版本任一未冻结时，Gateway所有方法都只允许返回稳定`unsupported`，不得触达Provider。

## 5. 反例与兼容影响

- `Retrieve(query, scope)`只有业务参数，没有Operation/Attempt/Permit/Enforcement坐标；
- Begin后超时直接重试，或换Provider后沿用原Attempt；
- Provider success直接生成Context Contribution或Memory事实；
- 远程正文流直接进入Frame，未先形成Owner本地exact ref并完成Settlement；
- Evidence失败后仍Apply成功并向Context宣称closed；
- Runtime Settlement引用A DomainResult，Memory Apply却接受同ID不同revision/digest的B；
- 用Current Reader的`InspectAttempt`替代Gateway的`InspectOriginalAttempt`。
- 把Tool G6A的closed verdict改个kind后用于Retrieval；
- 把Checkpoint V5的Evidence/Settlement坐标套给remote-query；
- 只冻结Retrieval Evidence但未冻结Applicability/Settlement，仍允许一次Provider探测调用；

这是加法版本Delta，不修改Runtime现有Port，不建立私有兼容接口，不预选Provider、向量库、远程存储、RPC、进程拓扑或SLA。
