# Knowledge Retrieval Domain Gateway V1 Port Delta

状态：**公共治理Delta，等待Knowledge/Application/Runtime/Review/Evidence/Asset联合Review；当前所有远程路径unsupported且Provider/Resolver调用数必须为0**。

## 1. 用例与Owner

该Delta仅承载`praxis.knowledge/remote-query`、`praxis.knowledge/remote-resolve`与`praxis.knowledge/remote-content-read`。Knowledge Owner拥有查询、Citation/Coverage、Retrieval Observation/Result、DomainResultFact、Inspect/CAS与ApplySettlement；Asset Owner仍拥有原始Artifact字节；Application负责跨域编排；Runtime拥有Operation/Attempt/Permit/Begin/Fence/Settlement；Review与Evidence保持独立Owner。Provider命中、解析结果、Trust标签和远程正文均只是Observation。

当前G6A closed applicability/evidence/settlement matrix只证明Tool类动作，不覆盖Knowledge Retrieval/Resolver。Checkpoint V5仅属于Checkpoint Owner语义，不得复用或包装成Retrieval合同。本Delta必须等待相应Owner联合冻结三个**retrieval-specific additive version**：Retrieval Applicability、Retrieval Evidence、Retrieval Settlement；版本号、schema、license字段映射和兼容规则当前均待定。不得自行借用Tool或Checkpoint版本。本地`KnowledgeContextSourceCurrentReaderV1`不受该远程门禁影响。

## 2. 拟议Port与治理坐标

合同名：`praxis.knowledge/retrieval-domain-gateway/v1`。

```text
KnowledgeRetrievalDomainGatewayV1
  Retrieve(KnowledgeGovernedRetrievalRequestV1)
    -> KnowledgeRetrievalDispatchReceiptV1

  ReadRemoteContentExact(KnowledgeGovernedRemoteContentRequestV1)
    -> KnowledgeRemoteContentDispatchReceiptV1

  InspectOriginalAttempt(KnowledgeRetrievalEffectAttemptCoordinateV1)
    -> KnowledgeRetrievalEffectInspectionV1

  ApplySettlement(KnowledgeRetrievalApplySettlementRequestV1)
    -> KnowledgeRetrievalApplySettlementResultV1
```

每个Effect请求必须canonical绑定：

- OperationSubject、Operation ID/revision/digest、Domain Intent、Reservation；
- Admission、Review Verdict、Permit及Permit expiry；
- Begin、Delegation/Prepare、Prepare Enforcement、Execute Enforcement；
- 原Effect Attempt ID/revision/digest、Idempotency Key、Conflict Domain；
- Identity/Fence epoch、Authority、Policy、Scope、Purpose、Sensitivity、AllowedLicenses、预算与NotAfter；
- Provider/Remote Target或Asset Resolver exact ref、Payload ref/digest；
- Query/View/Expected Published Snapshot/Pointer；远程正文另含Package/Record/Source/Content exact refs。

缺少坐标、摘要不一致、Permit/Review过期、Fence漂移、License不允许或双Enforcement未通过，必须在远程执行前拒绝。

## 3. 固定执行顺序与输出

```text
Domain Intent/Reservation
 -> Admission
 -> Runtime Permit
 -> Runtime Begin
 -> Delegation/Prepare
 -> prepare Enforcement
 -> execute-point Enforcement
 -> Provider/Resolver Execute或Inspect
 -> Provider Observation
 -> Knowledge Owner Inspect并持久化Retrieval Observation/Result
 -> Knowledge DomainResultFact CAS
 -> retrieval-specific Evidence append（待新加法版本）
 -> retrieval-specific Runtime Operation Settlement(ref only，待新加法版本)
 -> Knowledge ApplySettlement CAS
 -> Current Reader复读
```

Dispatch Receipt不证明远程成功。Provider/Resolver响应只形成`ProviderObservationRef`；正式Observation/Result必须由Knowledge Owner重算canonical、验证Source/License/Citation/Coverage并CAS。

`DomainResultFact`精确绑定Operation、原Attempt、Provider Observation、Observation/Result exact refs、Source/Package/Snapshot/Pointer、CAS before/after、Evidence requirement、Residual及canonical digest。`ApplySettlement`必须精确绑定同一DomainResultAssociation与Runtime Settlement exact ref，错Operation/Attempt/DomainResult/Settlement一律拒绝。

远程正文不得直接流入Context Frame。它必须先形成Knowledge Owner本地Content exact ref及locality proof，完成DomainResult/Settlement/Apply后才可由Current Reader读取。

## 4. Unknown、Evidence与支持门禁

Begin后丢回包只能调用`InspectOriginalAttempt`，必须复用原Operation/Attempt/Permit/Prepare/Enforcement/Provider或Resolver坐标；不得换Source、Provider、Snapshot、Payload或License条件。远程Inspection若本身是Effect，只能使用Runtime批准的非递归Inspection Operation并关联原Attempt。

以下能力没有联合Review YES前，所有远程路径保持`unsupported`且Provider/Resolver调用数为0：

1. Application可传递Operation/Attempt/Permit及双Enforcement exact坐标；
2. Provider/Resolver Observation可持久化并由Knowledge Owner独立Inspect；
3. Evidence Owner冻结retrieval-specific additive Evidence applicability/schema/version，可引用精确DomainResultFact；
4. Runtime/Application Owner冻结retrieval-specific additive Applicability与Settlement schema/version，使Settlement可ref-only关联DomainResult；
5. Knowledge ApplySettlement可验证Association并闭表；
6. Asset/License边界可证明远程正文已合法进入Knowledge Owner本地State Plane。

未完成Evidence、Runtime Settlement和ApplySettlement闭表的远程结果不得交给Current Reader，不得伪装成本地命中、Complete Coverage或可引用Citation。

Tool G6A closed matrix、通用Evidence表述或Checkpoint V5不能解除该门禁；三个retrieval-specific版本任一未冻结时，Gateway所有方法只能稳定返回`unsupported`，不得执行Provider/Resolver探测。

## 5. 反例与兼容影响

- `Retrieve(query, licenses)`无Operation/Attempt/Permit/Enforcement坐标；
- Begin后超时换Provider/Source重试并沿用原Attempt；
- Provider命中直接变成Knowledge Fact、Citation或Context Fragment；
- Resolver正文直接流入Frame，未形成本地exact ref与Settlement；
- Source withdraw或License收紧后仍接受旧Provider Observation；
- Evidence失败却向Context宣称closed；
- Runtime Settlement绑定A，Knowledge Apply接受同ID不同revision/digest的B；
- 用Current Reader的`InspectAttempt`替代Gateway的`InspectOriginalAttempt`。
- 将Tool G6A closed applicability换kind后用于Knowledge Retrieval；
- 将Checkpoint V5 Evidence/Settlement坐标用于remote-resolve；
- Retrieval Evidence版本获批但Applicability或Settlement仍未冻结时允许Provider探测；

这是加法版本Delta，不修改Runtime/Asset现有Port，不建立私有兼容接口，不预选Provider、向量库、图数据库、远程索引、RPC、进程拓扑或SLA。
