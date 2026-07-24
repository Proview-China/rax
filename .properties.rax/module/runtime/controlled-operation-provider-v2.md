# Runtime Controlled Operation Provider V2模块说明

## 1. 作用

本切面把Tool Action进入actual Provider不可逆execute admission前的最后治理窗口缩窄、线性化并可审计化。它不消除跨Owner撤销窗口，也不把Runtime事实写入与Provider外部副作用声明为分布式原子或物理exactly-once。

## 2. 公共合同

- `runtime/ports`唯一拥有五个V2新增Route中立类型：DeclarationRef、ConformanceRef、CurrentRef、CurrentProjection、CurrentReader；既有MatrixKeyV3继续复用；
- Harness Assembly仍是Declaration/Conformance/Route Current事实语义Owner，Runtime不定义第二套Harness事实；
- Tool Adapter只持`ControlledOperationProviderPortV2`，Application不直调，双方都不持Entry FactPort或raw Provider；
- public Inspect key由Operation、derived EntryID、StableKeyDigest与ExpectedRequestDigest闭合，caller不能替换Prepared或Attempt；
- Result只允许`entered|unknown|observed|rejected_no_effect`，不携kernel Authorization，不等于Observation、DomainResult、Settlement或Outcome。

## 3. 双水位与Entry线性化

- Prepared与`PreparedSemanticSnapshot`绑定legacy V3 Permit revision/digest；
- Request的execute Attempt、Boundary与execute Enforcement绑定V4 Permit Fact current revision/digest；
- Entry ID由immutable Operation/Effect/Attempt/Prepared stable key派生，Boundary revision、fresh Checked和NotAfter不进入唯一execution identity；
- 只有真正完成`absent -> entered`的本调用获得不可复制、不可持久opaque claim，并可在同call stack进入kernel Runner；
- existing、lost-create或并发已推进Fact只Inspect。相同immutable request允许更高合法revision/current closure/终态恢复，changed-content或非法旁支Conflict，绝不重拿claim或重调Provider。

## 4. Current与历史真实性闭包

Gateway在Entry创建前fresh复读Route Current、Generation/Handoff、BindingSet、active-route、七个Binding、Effect/Intent、Prepared、Evidence Policy、Applicability Policy、execute Enforcement、Handoff/Qualification/Scope和Boundary。

Entry Fact持久冻结七个角色与Route ref的exact映射，并要求每个Binding的BindingSet ID/revision/digest/semantic digest一致。`EnteredUnixNano`必须早于Unified NotAfter，所有fresh projection必须满足`Checked/Issued <= Entered < Expires/NotAfter`；这证明历史进入时真实有效，不把已过期历史Fact重新升级为current执行资格。

## 5. 恢复边界

- lost Entry create reply即使Inspect看到exact entered，也不能恢复opaque claim，本次Provider调用数为0；
- lost Provider reply只按Entry内原Prepared/Attempt stable key调用read-only Inspect；
- unknown必须零AdmissionReceipt、零Observation sidecar；能证明no-effect才进入`rejected_no_effect`，否则保持unknown；
- CAS lost reply可接受相同immutable request的同状态或合法单调后继，但只Inspect，不重Runner；低revision、回退或旁支状态拒绝。

## 6. 验证

Owner与中央已实际通过：

```text
go test -count=1 ./tests/ports ./tests/fakes -run ControlledOperationProvider
go test -count=100 ./tests/ports ./tests/fakes -run ControlledOperationProvider
go test -race -count=20 ./tests/ports ./tests/fakes -run ControlledOperationProvider
go test -count=1 -shuffle=on ./...
go test -race -count=1 -shuffle=on ./...
go vet ./...
gofmt -l <本纵切相关Go文件>        # 无输出
git diff --check -- .              # PASS
```

第三轮独立代码审计最终为`YES`，P0/P1/P2均为0。反例覆盖Route public shape/AST冻结、七Binding角色与集合漂移、entered-time过期/future水位、恶意Store重Seal、lost reply、64并发单claim和immutable/progressed recovery。

## 7. 限制

当前只提供Runtime reference store、fake transport、隔离测试与public Conformance；没有production composition root、生产持久backend、真实Provider transport、availability、SLA或物理exactly-once声明。V1保持fixture-only，V1/V2生产路由互斥仍依赖Harness active-route current proof。

设计入口：[Controlled Operation Provider V2](../../design/runtime/controlled-operation-provider-v2/README.md)。
