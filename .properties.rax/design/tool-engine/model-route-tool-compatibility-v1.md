# PD-TM-05：Model Route Tool Compatibility V1联合候选

## 1. 结论

Tool Owner已经能把exact current `ToolSurfaceManifestCurrentV1`确定性编译成Model Invoker公开
`[]modelinvoker.Tool`，但这只证明neutral表达合法。live Model Invoker没有公开事实证明某条
`RouteID + adapter + protocol + model`当前接受名称、Schema、strict、ToolChoice和parallel组合。

因此production Provider调用继续NO-GO，直到Model Owner发布本文件定义的additive historical
Fact、current Projection、Prepared Association与actual-point exact Reader。现有
`Request.Validate()`、`CapabilityToolCalling`、`CapabilityParallelToolCalling`、
`RouteSelection`或Provider回包均不得包装成该事实。

本文件是Tool Owner提交的联合候选，不是Model Owner已冻结合同，也不授权修改Model实现。

## 2. live证据

可直接复用的public nominal：

- `modelinvoker.PreparedModelInvocationRefV1`、`PreparedModelInvocationFactV1`；
- `modelinvoker.PreparedModelInvocationCurrentRefV1/ProjectionV1`及两个exact Reader；
- `runtimeports.ModelPreDispatchAssemblyCurrentRefV1/ProjectionV1/ReaderV1`；
- `modelinvoker.Tool`、`ToolChoice`、`Protocol`、`ProviderID`；
- `upstream.RouteID`；
- Tool侧`CompiledModelToolsV1.Digest`只由Tool Owner验证，Model不得import `tool-mcp/surface`。

不能复用或伪造：

- `PreparedModelInvocationFactV1`只有`RequestToolsDigest`、`RouteDigest`、`ProfileDigest`和
  `ActualToolSurfaceDigest`，没有ToolChoice/parallel控制摘要或Compatibility Ref；
- `RouteSelection.EvidenceDigest`是调用审计值，不是版本化current Ref；
- `CapabilityContract`只表达粗粒度支持，不证明具体工具定义可映射；
- provider/internal dialect与厂商SDK DTO不是公共Port；
- terminal Tool Call Observation只证明模型输出，不倒推请求兼容性。

## 3. Model Owner additive historical Fact

正式名称候选：`PreparedModelToolRouteCompatibilityFactV1`。

### exact Ref

`PreparedModelToolRouteCompatibilityRefV1`至少包含：

| 字段 | 约束 |
|---|---|
| `ContractVersion` | Model Owner版本化常量 |
| `ID/Revision/Digest` | create-once immutable exact Ref；Revision 1 |
| `Prepared` | exact `PreparedModelInvocationRefV1` |
| `UnifiedRequestDigest` | 必须等于`Prepared.UnifiedRequestDigest` |

ID由`Prepared`完整坐标确定性派生；Digest覆盖下述完整Fact。相同Prepared同内容幂等，任一内容
变化Conflict，不能因adapter升级原位重写。

### Fact字段

| 字段 | exact语义 |
|---|---|
| `Ref` | 上述historical Ref |
| `Prepared` | exact Prepared历史坐标 |
| `Assembly` | exact `ModelPreDispatchAssemblyCurrentRefV1`，只证明本次Assembly闭包 |
| `RouteID` | exact `upstream.RouteID` |
| `RouteDigest/ProfileDigest` | 与Prepared Fact逐字段相等 |
| `AdapterID/Protocol/Model` | Model Owner从Route/Catalog和sealed request解析，调用方不能选择 |
| `RequestToolsDigest` | 与Prepared Fact相等，由Model canonical request重算 |
| `ToolControlDigest` | Model Owner canonical绑定ToolChoice、ParallelToolCalls及所有影响映射的public request控制字段 |
| `ActualToolSurfaceDigest` | 与Prepared Fact及Assembly ToolSurface闭包交叉验证 |
| `Decisions` | 按请求工具顺序逐项的typed决定与Residual |
| `CompatibilityDigest` | 绑定完整决定、adapter rule version与Route Evidence |
| `CreatedUnixNano/NotAfterUnixNano` | historical形成时间与资格绝对上界；不是retention TTL |

每个Decision固定为`exact|transformed|degraded|rejected`，至少携tool ordinal、name、完整
Model-owned request-tool digest、provider-mapped digest、规则版本及有界Residual。`transformed`只允许
表达转换，不得改变Tool Effect、Authority、Review、Scope、Budget、Credential或Sandbox语义；
`degraded/rejected`在V1 production链均Fail Closed，Provider调用为零。

## 4. current Projection与Prepared Association

### current

`PreparedModelToolRouteCompatibilityCurrentRefV1/ProjectionV1`至少绑定：historical Ref、exact
Prepared Current Ref、RouteID、RouteDigest、ProfileDigest、AdapterID、Protocol、Model、
RequestToolsDigest、ToolControlDigest、CompatibilityDigest、Route/Catalog Evidence digest、adapter
rule version、Checked、Expires、NotAfter和ProjectionDigest。

`ValidateCurrent(expectedRef, now)`必须满足：

- full Ref逐字段exact；`Checked <= now < Expires <= NotAfter`；
- `Expires`不晚于Prepared Current、Assembly Current、Route Evidence、adapter lifecycle和caller
  deadline的共同最早上界；
- current刷新只能返回同一historical Ref与同一CompatibilityDigest；规则、Route、adapter或请求变化
  必须新Prepared Invocation/新Fact，不能刷新成“等价”对象；
- clock rollback、Unavailable、Unknown、expired或任一digest漂移均Fail Closed。

### Association

不修改live `PreparedModelInvocationFactV1` canonical。Model Owner新增create-once
`PreparedModelToolRouteCompatibilityAssociationV1`，以exact Prepared Ref为唯一稳定key，返回
historical Ref与current Ref。Reader候选：

```go
type PreparedModelToolRouteCompatibilityReaderV1 interface {
    InspectPreparedModelToolRouteCompatibilityV1(
        context.Context,
        PreparedModelToolRouteCompatibilityRefV1,
    ) (PreparedModelToolRouteCompatibilityFactV1, error)
    InspectCurrentPreparedModelToolRouteCompatibilityV1(
        context.Context,
        PreparedModelToolRouteCompatibilityCurrentRefV1,
    ) (PreparedModelToolRouteCompatibilityCurrentProjectionV1, error)
    InspectPreparedModelToolRouteCompatibilityAssociationV1(
        context.Context,
        PreparedModelInvocationRefV1,
    ) (PreparedModelToolRouteCompatibilityAssociationV1, error)
}
```

第三个方法不是latest查询：Prepared Ref本身是完整exact create-once key；同Prepared只允许一个
Association。NotFound只表示尚未形成，Unavailable/Indeterminate不得当NotFound重建。

## 5. 形成与调用顺序

1. Tool Owner exact-current读取Surface，编译neutral tools并生成Tool-owned Compiled digest；
2. Harness/host把完整tools放入Model public Request，不传provider/internal DTO；
3. Model PurePrepare从sealed request、Route/Catalog、adapter rule version和Runtime neutral Assembly
   current形成historical Compatibility Fact及Prepared Association；
4. Model Commit Gate前S1读取historical/current/Association，拒绝任一`degraded|rejected`；
5. 每次Provider attempt/Open/Stream/continuation actual-point前，Model Gateway按同一Association
   复读同一historical Ref与current Ref，并与Prepared Current、Assembly Current、Route selection、
   adapter factory version和caller deadline逐字段交叉；
6. S2通过后才能进入Provider；Provider回包仍只是Observation，不能升级Compatibility Fact；
7. lost read reply只Inspect同一Prepared Association/Fact/current，不重新Prepare或换Route。

该流程不是Runtime Effect；Compatibility计算与Inspect均为Model Owner本地只读/确定性准备工作。
真正Provider调用仍走Model/Runtime既有治理与actual-point边界。

## 6. Owner与import边界

- Model Invoker Owner：Fact/current/Association nominal、canonical、Repository、adapter规则与Readers；
- Tool Owner：Surface、Tool Definition Material、portable compile与Compiled digest；只消费Model public
  Reader，不实现或复制Model事实；
- Runtime Owner：唯一neutral Assembly current nominal；不拥有Model兼容语义；
- Harness/host：composition与Gate调用，不创建Model或Tool事实；
- Application/Context：只消费结果，不import Model/Tool实现。

Model不得import `tool-mcp/**`；Tool不得import`model-invoker/internal`、provider包或厂商SDK；Harness
不得import Tool/Model实现。允许依赖方向是双方各自实现依赖Runtime public neutral contracts，由宿主
composition root注入，不形成SCC。

## 7. 错误、恢复与硬反例

closed分类候选：Invalid、Conflict、NotFound、Unavailable、PreconditionFailed、Indeterminate与原样
`context.Canceled/DeadlineExceeded`。至少覆盖：

1. typed-nil Reader、nil/canceled context：零Provider；
2. Prepared/Current/Assembly/Route/Profile/Adapter/Protocol/Model任一漂移：零Provider；
3. RequestToolsDigest或ToolControlDigest换字节：零Provider；
4. 同名Tool换Schema/strict/description或顺序：零Provider；
5. `CapabilityToolCalling`为supported但Decision rejected：零Provider；
6. Compatibility过期、clock rollback或adapter rule version漂移：零Provider；
7. S1通过后S2换Route/adapter/evidence/current Ref：零Provider；
8. Provider返回成功但没有pre-dispatch Compatibility闭包：不能补写Fact冒充已验证；
9. lost reader reply只能exact Inspect，不能重新Resolve到另一Route；
10. 64同Prepared并发只形成一个Association/Fact；同ID换内容Conflict；不同Prepared可并行；
11. Tool/Harness导入Model internal、Model导入Tool实现或复制Runtime neutral type：Conformance失败；
12. `degraded`被当success、Residual丢失或表达转换改变治理字段：Fail Closed。

测试门：unit、whitebox、blackbox、fault、Conformance、ordinary×100、race×20、full ordinary/race、
vet、gofmt、import boundary与diff-check。

## 8. 兼容与迁移

- additive，不修改`PreparedModelInvocationFactV1`、现有Route Gateway或Provider接口；
- 无Tool请求的Prepared Invocation可显式标记`not_applicable`，不得伪造空兼容Fact；
- owner-local portable compile保持YES；production“该Route可调用该Surface”保持NO-GO；
- 联合终审若选择在未来`PreparedModelInvocationV2`内嵌Compatibility Ref，可迁移Association，但
  V1历史Fact不可重写，双版本不得type-pun。

## 9. 当前P0/P1/P2

- P0：Model public historical/current/Association nominal与Readers尚未落盘；
- P0：Prepared V1未绑定ToolControl/Compatibility，Model Gateway actual-point未复读；
- P0：Route/Catalog Evidence没有可复用的public exact-current Ref；Model Owner必须在上述Projection
  中发布无损闭包，Tool不得自造；
- P1：stream/Open/continuation所有Provider路径需要证明统一Gate；
- P2：production composition root和持久Backend未落。

结论：`joint_candidate_no_go`。等待Model/Runtime/Harness联合字段审定与用户确认；Tool侧不写
Model compatibility Go，也不把portable profile升级成production Route保证。
