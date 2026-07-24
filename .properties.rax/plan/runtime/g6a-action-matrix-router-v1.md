# Runtime G6A Action Matrix/Router V1实施计划

## 1. 状态与边界

- 对应设计：[G6A Action Matrix/Router V1](../../design/runtime/g6a-action-matrix-router-v1/README.md)；
- 当前状态：中央交叉实现性复核发现Boundary Reader公共缺口，上一轮独立Review不作为最终裁决；本修订等待重新联合评审，联合`YES`前不实现代码；
- Runtime保持冻结；本计划不修改OperationScope Evidence V3、Dispatch V4.0、Enforcement 4.1或Settlement V4冻结公共合同；
- 本计划不授权Harness、Application、Tool、Context、Model Adapter或生产composition root。

## 2. 固定产出

联合`YES`后，Runtime最小实现只产出：

1. 唯一closed matrix：`run + praxis.tool/execute + praxis.tool/single-call-action-v1`；
2. Generation与Run/Session/Turn/Action/Context五维全部required验证；
3. source四字段无损nominal projection；
4. 基于现有`OperationScopeEvidenceApplicabilityCurrentReaderV3`的封闭Kind路由与公共current投影验证；
5. Runtime-neutral `OperationProviderBoundaryRefV1`、current projection和注入式只读Reader；
6. prepare/execute独立Evidence V3与Boundary Reader public-only Conformance；
7. 零写、零Provider、lost-reply和current漂移反例。

不产出Applicability Fact Store、万能Hook、N>1、Checkpoint、Context Refresh、Continuation、Turn推进、生产Backend或SLA。

## 3. 前置依赖门

以下公共资产全部可读且联合`YES`前，不进入Runtime代码：

- Application中立Session/Turn source coordinates与窄Port；
- Harness distinct source coordinates、`CommittedPendingActionReader`和`runtimeadapter` current Reader；
- Tool `ActionCandidateV2` current reader、由Tool Owner Adapter实现的Runtime-neutral `OperationProviderBoundaryCurrentReaderV1`、DomainResult与settled `ToolResultV2`；
- Context ParentFrame current reader；
- Model Projection Exact Reader；当前仍待Model Owner资产`YES`，缺失即NO-GO；
- Runtime真实Run current、Generation-Binding current、Evidence V3、Dispatch V4.0、Enforcement 4.1及Settlement V4。

当前无production composition root。实现阶段只能由显式test fixture注入全部Reader；宿主Owner生产root留到G6B前独立设计和验收。

## 4. 实施波次

### G6A-P0：联合合同冻结

- [ ] 中央联合评审README、contracts、drawio、test matrix与本计划；
- [ ] 确认Action维度只引用Tool `ActionCandidateV2` exact source；
- [ ] 确认Session/Turn Application neutral coordinate与Harness Owner source coordinate映射；
- [ ] 各Owner冻结exact namespaced Kind与只读Reader版本；不得使用wildcard或动态自注册；
- [ ] Model Projection Exact Reader资产获得Owner与中央`YES`；
- [ ] 确认没有Runtime Applicability Fact Create/Inspect Delta。
- [ ] 确认Boundary Ref/Projection/Reader是Runtime-neutral只读合同，Tool Adapter实现Reader，Runtime无Boundary Fact/Store写口。

退出：中央联合`YES`。否则保持Runtime冻结。

### G6A-P1：Runtime closed matrix与nominal projection

- [ ] 仅新增唯一Action矩阵；现有activation matrix保持原义；
- [ ] Generation与五维required逐项验证；
- [ ] 实现四字段逐字段无损projection，不重seal、不生成新identity；
- [ ] unknown/custom Operation、Effect、Profile、Kind与缺维度全部零写拒绝；
- [ ] unit/property覆盖type-pun、nil/empty、changed ID/revision/digest。

### G6A-P2：Owner-current Router

- [ ] 按`dimension + exact Kind + Owner contract version`建立封闭路由；
- [ ] 调用既有`OperationScopeEvidenceApplicabilityCurrentReaderV3`；
- [ ] exact验证Fact、ExecutionScopeDigest、Current、Checked/Expires和Projection digest；
- [ ] Reader unavailable、路由缺失/重复、source漂移、TTL crossing与clock rollback全部Fail Closed；
- [ ] Runtime不复制Harness、Tool或Context领域检查。

### G6A-P3：Evidence phase、Tool boundary与受控Provider门

- [ ] prepare/execute各自Issue、Handoff、Consume且identity/source sequence/4.1 phase独立；Consumption只在对应响应/Observation后形成；
- [ ] phase交换、复用、遗漏或重复时零Provider；
- [ ] Tool Owner在exact/current execute Enforcement与同Attempt execute Handoff后CAS Watermark到`provider_boundary_crossed`并单调绑定两个public refs；Runtime不写该Watermark；
- [ ] 在候选`ports/operation_scope_evidence_action_v3.go`冻结`OperationProviderBoundaryRefV1`、current projection和只读Reader；Projection exact绑定Operation/Scope digest、Runtime Attempt、execute Enforcement/Handoff、Stage与TTL；
- [ ] Tool Owner Adapter实现Reader并复读自身Watermark；Runtime不import Tool、不创建Boundary Fact/Store；
- [ ] 受控Provider seam以exact Boundary Ref调用Reader并逐字段交叉校验；missing/unavailable/NotFound、type-pun、漂移、跨Attempt或过期时零Provider；
- [ ] boundary CAS成功即可能已调用，随后至多一次Provider调用；lost reply/崩溃只Inspect原Attempt/Observation；
- [ ] Provider Observation不得成为DomainResult或Settlement；
- [ ] lost reply只Inspect原canonical ID，UnknownOutcome不重派Provider。

### G6A-P4：隔离fixture与停止线验收

- [ ] 显式test fixture注入Run/Harness/Tool/Context/Model/Generation Readers；
- [ ] N=1纵向链输出settled `ToolResultV2 + current V4 Inspection + public Association Inspect`；
- [ ] Capability、Context Refresh、Continuation与Turn推进调用计数均为0；
- [ ] N>1、万能Hook与Checkpoint保持unsupported；
- [ ] Conformance明确`ProductionClaimEligible=false`且不宣称生产root、Backend或SLA。
- [ ] Conformance只依赖Runtime公共Reader接口、Boundary Ref/Projection和Provider计数seam，不获得Tool Watermark写口。

## 5. 测试与门禁

实现阶段必须按[测试矩阵](../../design/runtime/g6a-action-matrix-router-v1/test-matrix.md)覆盖unit、whitebox、blackbox、fault、lost reply、64并发与race，并执行：

```text
go test -count=1 ./...
go test -count=1 -shuffle=on ./...
go test -count=1 -race ./...
go vet ./...
gofmt -l .
git diff --check -- .
```

资产阶段只执行Markdown相对链接、drawio XML、冻结术语与stale claim检查，不把上述代码命令标记为已通过。

## 6. 完成条件

- 唯一Action矩阵与五维required在Runtime和Owner资产间一致；
- nominal projection不创作Fact、identity、digest或TTL；
- 所有Reader按Kind/source exact current，缺失或漂移零写；
- prepare/execute不能交换或复用；Provider接触顺序必须是4.1 execute current → execute Handoff current → Tool boundary CAS → Runtime-neutral Boundary Reader current proof → 至多一次调用，Consumption在响应后；
- 输出硬停于G6A三项，不启用G6B能力；
- 仍明确没有production composition root、生产Backend或SLA。

完成G6A-P0联合`YES`前，本计划不得转为代码实施。
