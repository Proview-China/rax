# Evidence Subject Current V1 Port Delta

状态：**第七候选第一独立资产审计YES（P0/P1/P2=0/0/0）；asset candidate等待Continuity第二独立资产审计，implementation仍NO-GO，不写Go**。

## 1. Live复用

| Live能力 | 复用方式 | 不能承担的语义 |
|---|---|---|
| `EvidenceSourceRecordReaderV2` | 仅由Runtime Evidence Owner在具名`EvidenceSubjectRecordRegistrationCurrentReaderV1`实现内用`InspectBySource + InspectRecord`完成immutable Record双读 | 不直接注入Gateway/Continuity；不承担source/policy current、readability、retention、Tombstone absence |
| `EvidenceSourceRegistrationFactV2` | projection绑定exact revision/full digest/configuration digest/state/TTL | subject-current index或absence watermark |
| `EvidenceSourcePolicyReaderV2` | Runtime Gateway复读exact policy current | Continuity不得直接拼接Policy与Record |
| `ExecutionScopeFactReaderV2` | S1/S2复读exact `ExecutionScopeCurrentFactV2`，回扣CurrentScope/Scope/source refs/watermark/state/TTL | 不信任Projection自述Scope current |
| `ProviderBindingCurrentnessPortV2` | 分别复读Producer与Consumer Reader Binding current，唯一映射exact refs | 不创建Binding/Grant，不使用caller TTL |
| `AuthorityFactReaderV2` | S1/S2复读exact `DispatchAuthorityFactV2`，回扣Authority/Scope/ActionScope/state/TTL | 不把Readability Policy或Reader capability当Authority |
| `EvidenceTombstoneFactV2` | 派生typed Tombstone Ref，保留append-only历史 | 空Tombstone/NotFound不能证明absence |
| `EvidenceLedgerFactPortV2` | 仅Runtime Evidence Owner内部兼容原语 | 禁止注入Continuity；不能作为R-CTY-06 Reader |
| `CheckpointEvidenceSourceCurrentProjectionV1` | 不复用 | checkpoint专用Scope/Qualification不能type-pun为通用subject current |

## 2. Additive公共候选

- `EvidenceSubjectKeyV1`；
- `EvidenceSubjectProjectionRefV1`；
- `EvidenceSourceRegistrationRefV1`；
- `EvidenceSubjectReaderCapabilityRefV1`；
- `EvidenceSubjectReaderBindingRefV1`；
- `EvidenceSubjectConsumerAssociationRefV1`、`EvidenceSubjectConsumerAssociationCurrentProjectionV1`与窄current Reader；
- `EvidenceTombstoneRefV1`；
- `EvidenceTombstoneAbsenceRefV1`；
- `EvidenceReadabilityPolicyRefV1`；
- `EvidenceSubjectCurrentIndexRefV1`；
- `EvidenceSubjectCurrentProjectionV1`；
- `EvidenceSubjectCurrentValidationRequestV1`；
- `EvidenceSubjectCurrentLookupRequestV1`；
- `EvidenceSubjectCurrentSnapshotV1`；
- `EvidenceSubjectRecordRegistrationCurrentRequestV1/ResultV1/ReaderV1`；
- `EvidenceSubjectPresenceReadabilityCurrentRequestV1/ResultV1/ReaderV1`；
- `EvidenceTombstonePresenceV1`与`EvidenceSubjectReadabilityV1`闭表；
- `EvidenceSubjectMutationKindV1`、`EvidenceSubjectMutationRequestV1`与`EvidenceSubjectMutationKeyV1`；
- `EvidenceSubjectMutationCommitV1`；
- `EvidenceSubjectCurrentReaderV1`。

核心identity与live装配证明的冻结JSON shape为：

- `EvidenceSubjectProjectionRefV1{ProjectionID,Revision,SubjectKeyDigest,OwnerWatermark,Digest}`；
- `EvidenceSourceRegistrationRefV1{RegistrationID,Revision,FactDigest,ConfigurationDigest,SourceID,SourceEpoch}`；
- `EvidenceSubjectReaderCapabilityRefV1{Name,BindingRevision,GrantDigest,BindingCurrentProjectionDigest,IssuedUnixNano,ExpiresUnixNano}`；
- `EvidenceSubjectReaderBindingRefV1{Binding,BindingSetDigest,BindingSetSemanticDigest,BindingID,Capability}`。

`EvidenceSubjectCurrentIndexRefV1`冻结为`{IndexID,Revision,SubjectKeyDigest,PreviousProjection,CurrentProjection,OwnerWatermark,Digest}`。精确类型与snake_case JSON tag见[contracts](./contracts.md)。这些类型属于`runtime/ports`候选；Runtime拥有类型与current/readability语义。ProjectionID只由固定domain/version+SubjectKeyDigest派生，同ID revision严格`+1`；Projection Ref和Projection共享同一完整canonical digest。Projection不含Index Ref/digest，Index只在Projection完成seal后绑定其完整Ref。Continuity只实现消费，不定义alias、第二Store或第二canonical。

`EvidenceSubjectMutationRequestV1/KeyV1/CommitV1`是Runtime Evidence Owner内部候选，不加入Continuity-facing Reader。Request冻结ContractVersion/Subject/closed Kind/full expected refs及Registration|SourcePolicy|Tombstone|ReadabilityPolicy exact one-of payload；Key由命名`EvidenceSubjectMutationStableKeyInputV1`派生，并强制`MutationID == string(StableKeyDigest)`。首次create使用固定no-current哨兵，后续CAS必须携完整expected refs；RequestDigest、Subject/Kind、expected refs在Request→Key→Commit三段必须exact，用于lost-reply exact Inspect与三分恢复。

`EvidenceSubjectCurrentReaderV1`新增只读bootstrap lookup：`InspectEvidenceSubjectCurrentV1(ContractVersion+Subject+ExpectedConsumer+Scope+Record Policy)`按稳定IndexID返回sealed Snapshot；ExpectedConsumer只能与Gateway构造时注入的bound association current proof比对，不授权。association由host Assembly/Binding Owner发布，精确绑定principal/consumer/execution scope；Runtime Evidence Owner只持窄current Reader。S1/S2真实调用七组依赖，其中Record+Registration和Presence+Readability各用具名Request/Result/Reader，结构不兼容`EvidenceLedgerFactPortV2`。Readability Policy冻结exact SubjectKeyDigest+ExecutionScopeDigest+Consumer+AllowRead，三个新增current expiry与其他Owner上限共同取natural min-TTL。

`EvidenceSubjectMutationCommitV1`是同Owner immutable history，不是Continuity写Port。它完整绑定MutationKey/RequestDigest、expected Index/Projection与new Projection/Index；lost reply恢复必须Inspect exact Commit，并由immutable Index/Projection history证明其new Index仍current或为current祖先，不能只观察较高revision猜成功。

## 3. 实现候选落点

联合Review YES且用户授权后，最小独占路径建议为：

```text
ExecutionRuntime/runtime/ports/evidence_subject_current_v1.go
ExecutionRuntime/runtime/control/evidence_subject_current_v1.go
ExecutionRuntime/runtime/kernel/evidence_subject_current_gateway_v1.go
ExecutionRuntime/runtime/fakes/evidence_subject_current_store_v1.go
ExecutionRuntime/runtime/conformance/evidence_subject_current_v1.go
ExecutionRuntime/runtime/tests/{ports,control,fakes}/evidence_subject_current_v1_test.go
```

Reference store必须作为现有`EvidenceLedgerStoreV2`同一Owner实例、同一锁/事务的扩展；禁止独立Store/锁在Tombstone或Source mutation后异步追赶current index。

## 4. 兼容策略

- 不改Evidence V2/V3已有字段、digest或写方法；
- 新Reader不嵌入`EvidenceGovernancePortV2`，也不让raw Fact Port自动满足；
- 旧V2 Record/Tombstone可作为历史输入，但只有经新Owner原子publish形成的projection才具R-CTY-06 current资格；
- production composition在新Conformance与Owner原子Store落地前保持NO-GO；
- 不新增production backend/root，不承诺durability或SLA。

## 5. 唯一实施顺序

1. ports types/canonical/Validate与reader capability narrowing；
2. 同Owner store扩展：先seal immutable historical Projection，再以完整旧Index Ref CAS新的Current Index；首次revision=1，后续严格+1，absence watermark与mutation同一原子publish；
3. Kernel Gateway：构造时接受bound association ref+Reader，request只提供ExpectedConsumer；完成association principal/consumer/scope current验证后，对七组Owner current执行S1/S2 fresh双读；Record+Registration与Presence+Readability必须走具名窄Reader，Execution Scope、Producer Binding、Authority必须调用live既有Reader，Consumer Binding/Capability严格按live current projection九项映射派生；
   S1/S2/request/Projection/Index必须逐字段full-equal，不能使用caller或projection自嵌TTL；
4. public-only Conformance与adversarial tests；
   必须闭合C01无环canonical/首seal、R02四Owner exact、R07 closed readability/presence、R09/R10 history/current/ABA、T05 lost-reply三分与T06 full-ref CAS并发一胜；
5. Continuity adapter另行获批，仅注入Reader，不取得Fact Port。
