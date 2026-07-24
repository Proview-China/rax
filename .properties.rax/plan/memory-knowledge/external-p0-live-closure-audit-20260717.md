# Memory/Knowledge External P0 live closure audit

> 历史审计快照：本文件记录实现前的External P0=5，不再代表current truth。当前五项reference Delta均已实现并通过软件测试；production root与远程Gateway仍NO-GO。当前状态见[计划入口](./README.md)。

> 状态：**current_truth / implementation_decision_pending**。本审计只记录2026-07-17 live公共合同可复用面与仍需对应Owner发布的最小加法；不修改Harness、Application、Context或Runtime合同，不把候选当Owner YES。

## 1. 结论

External P0仍按既有审计口径登记为5，尚不能宣称系统G6B完成；但P0-1的live前提已经变化：Harness与Application已有可无损复用的Session/Turn exact nominal和current Reader，**不再需要第二个Turn Owner Store或Reader**。P0-1剩余工作应并入P0-3的Application Refresh映射与S1/S2复读，而不是新造公共Turn事实。

| 项 | live可复用合同 | 真正缺口 | 最小动作 |
|---|---|---|---|
| P0-1 Turn exact | Harness `CommittedPendingActionReaderV3`；`CommittedPendingActionCurrentV3.SessionApplicability/TurnApplicability/Turn/Checked/Expires`；Application `SingleCallSession/TurnCoordinateV1`和对应Applicability Source nominal | Context Refresh V1和未来Application Refresh request尚未携带这些exact坐标 | Harness-owned Adapter把V3 current无损投影到既有Application nominal；P0-3 request携带并复读，不新增Turn Reader |
| P0-2 TransitionProof | Context live三段Owner-local Refresh/Apply/Inspect、pending DomainResult/Manifest/Frame/Generation、single backend/lock与atomic Apply+Generation CAS | 无Context-owned proof nominal/ref/current Reader；Apply/Settlement/Result未exact绑定proof | Context Owner发布加法proof及reader，并将ProofRef加入Apply、local Settlement和Result canonical body |
| P0-3 Application三阶段Port | Application已有namespaced合同、S1/S2 current readers、CAS coordination及通用Operation V3 | 无Context Refresh专属prepare/apply/inspect公共Port | Application发布中立三阶段Port；只编排exact refs，Context Adapter实现它 |
| P0-4 双Adapter/root | Memory/Knowledge V2 Owner Reader已完成；两份Adapter候选已有stable/fresh、Binding和TTL设计 | 无获批Application Port、Context source cardinality、Binding与root | 前三项YES后，两Owner Adapter可并行；Context/root串行启用nonzero source |
| P0-5 knowledge_reference | Knowledge V2 exact source/citation/license/conflict chain已完成 | Context `FragmentKind`闭集无`knowledge_reference`，offline SDK明确拒绝 | Context Owner加法kind及Candidate exact binding；不复用`memory_recall`或artifact kind |

## 2. P0-1唯一无损映射

来源必须是同一次`CommittedPendingActionReaderV3.InspectCommittedPendingActionCurrentV3`返回的current投影：

```text
Harness Current V3.Run/ExecutionScopeDigest
 -> Application Run exact coordinate
Harness Current V3.SessionID/SessionRevision/SessionDigest
 + SessionApplicability{Kind,ID,Revision,Digest}
 -> existing Application SessionCoordinate + SessionApplicabilitySource
Harness Current V3.Turn
 + TurnApplicability{Kind,ID,Revision,Digest}
 -> existing Application TurnCoordinate{ID=TurnApplicability.ID,
      Ordinal=Current.Turn, Revision=TurnApplicability.Revision,
      Digest=TurnApplicability.Digest}
 -> existing Application TurnApplicabilitySource
Harness Current V3.CheckedUnixNano/ExpiresUnixNano/Digest
 -> Refresh S1/S2 fresh observation boundary
```

两个kind均已使用相同namespaced值：`praxis.harness/session`与`praxis.harness/turn`。映射必须验证：

```text
SourceTurn.ID == TurnApplicability.ID
SourceTurn.Revision == TurnApplicability.Revision
SourceTurn.Digest == TurnApplicability.Digest
SourceTurn.Ordinal == Harness Current.Turn
SourceTurn.Ordinal == settled Tool Execution.Turn == Context ExpectedCurrent.Turn
```

禁止把`uint32`自行哈希、从legacy字符串补revision/digest、仅调用`SessionCurrentReaderV4`、或让Memory/Knowledge/Context重建Harness坐标。Application不能import Harness；映射实现仍应位于Harness-owned `applicationadapter`，只返回Application公共nominal，避免依赖环。

## 3. P0-2最小Context加法

不重写现有Refresh状态机，只新增Context-owned：

1. `ContextTurnTransitionRequestV1`：ExecutionScope/Run、Session和SourceTurn exact coordinate、SourceOrdinal、ExpectedTargetOrdinal、Parent/ExpectedCurrent、RefreshAttempt、RequestedNotAfter及canonical digest；不得携带未来Frame/Generation refs。
2. `ContextTurnTransitionProofV1`：request exact ref/digest、childExecution、pending DomainResult/Manifest/Frame/Generation exact refs、StableSourceSetDigest、Created/Expires、state与canonical digest；proof不授current。
3. `InspectContextTurnTransitionProofV1`：只读原RefreshAttempt/ProofRef；missing、drift、expired、cancel、clock rollback闭集。
4. 将`TransitionProofRef`与`StableSourceSetDigest`加入`ApplyContextTurnRefreshRequestV1`、`ContextTurnRefreshApplySettlementV1`和applied `ContextTurnRefreshResultV1`的canonical exact binding。

唯一时序：`pending outputs seal -> proof seal -> Owner S2 -> Context atomic local Apply+Generation CAS -> publish`。S2或CAS失败时proof可保留diagnostic，但Frame/Generation不可current。

## 4. P0-3最小Application公共Port

Application只发布自己的neutral nominal，不import Context或Memory/Knowledge：

```go
type ContextTurnRefreshPortV1 interface {
    PrepareContextTurnRefreshV1(context.Context, ContextTurnRefreshPrepareRequestV1) (ContextTurnRefreshPreparedRefV1, error)
    ApplyContextTurnRefreshV1(context.Context, ContextTurnRefreshApplyRequestV1) (ContextTurnRefreshCurrentRefV1, error)
    InspectContextTurnRefreshV1(context.Context, ContextTurnRefreshInspectRequestV1) (ContextTurnRefreshInspectionV1, error)
}
```

字段最小闭集：Application Attempt、ExecutionScope/Run、既有Session/Turn exact coordinates、settled Tool result chain、Parent/ExpectedCurrent、Owner source envelope exact refs、Context pending/proof/apply/current exact refs、RequestedNotAfter、Residual和canonical digest。Application不创建Context Fact、不解释Owner body、不mint proof、不计算T+1；lost reply只Inspect原Application/Context attempt。

## 5. P0-5最小Context加法

- `FragmentKnowledgeReference = "knowledge_reference"`进入Context closed kind、Recipe、Candidate、Frame Fragment和SDK strict codec；
- Candidate只能携带Context物化后的bounded ContentRef以及Knowledge Adapter exact envelope，必须绑定Record/Package/Snapshot/Source/Citation/License/Conflict/Association set digests；
- Context在S1/S2间复读stable closure与fresh TTL，不把RetrievalResult直接当Frame；
- 不支持的Recipe/Route返回`unsupported_fragment_kind`或Residual，禁止降级为`memory_recall`、`artifact_reference`、`instruction`。

## 6. 实施顺序与退出门

```text
Harness V3 -> existing Application Session/Turn exact mapping
       ||
Context TransitionProof + knowledge_reference（Context Owner可并行）
       -> Application三阶段Port
       -> Memory/Knowledge Adapter（可并行）
       -> Context nonzero source/cardinality + cross-module fixture
       -> production-root前集成审计
```

每个Owner必须独立通过target ordinary100、race20、full ordinary/race、vet、gofmt、diff与import boundary；联合fixture另覆盖lost reply、S1/S2 drift、TTL crossing、cancel、wrong Turn/Proof/kind、Frame pending可见性和64并发。真实远程Retrieval、Provider、Resolver与物理Purge不因该链启用。

## 7. 待用户/联合Owner的一次性裁决

1. 接受P0-1复用Harness V3 + Application既有Session/Turn nominal，不再新建Turn Reader；
2. 接受P0-2最小Context proof加法及Apply/Settlement/Result exact绑定；
3. 接受P0-3三阶段Port语义与依赖方向`Harness adapter -> Application neutral contract <- Context adapter`；
4. 接受P0-5独立`knowledge_reference`kind及完整Knowledge exact chain；
5. 只在1-4独立审计YES后启用P0-4双Adapter和nonzero来源。

上述五项未获确认前，Memory=0、Knowledge=0，Context/Application/root保持NO-GO。
