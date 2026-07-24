# Model Tool内容物化与neutral组装V1

## 1. 目的与边界

本切片不新增厂商Tool、不复制OpenAI/Anthropic/Gemini/Qwen DTO。Tool Owner把
`ToolSurfaceManifestCurrentV1`中已冻结的`ModelName + InputSchema Ref +
DescriptionDigest`解析为Model Invoker公开的`modelinvoker.Tool`；随后由Model
Invoker既有provider adapter完成各厂商协议映射。

该流程是本地纯读和确定性组装，不产生Effect，不联网，不授予执行权，也不修改
Surface中的Effect、Admission、Scope、Review、Credential、Sandbox或Fence语义。

## 2. ToolDefinitionMaterialV1

`ToolDefinitionMaterialRefV1`字段固定为：

- `ContractVersion`、稳定`ID`、`Revision=1`、`Digest`；
- exact `Tool ObjectRef`；
- exact `runtimeports.SchemaRefV2`；
- exact `DescriptionDigest`。

ID和Digest只由上述稳定坐标canonical派生。`ToolDefinitionMaterialV1`另携
`Description`与`InputSchema` bytes；验证必须满足：

- `sha256(Description bytes) == DescriptionDigest`；
- `sha256(InputSchema bytes) == InputSchema.ContentDigest`；
- Schema是单一JSON object、无尾随数据且不超过`MaxOpaqueInlineBytes`；
- 所有返回bytes均deep-copy。

Repository只有`EnsureExactToolDefinitionMaterialV1`与
`InspectExactToolDefinitionMaterialV1`。同Ref同内容幂等，同Ref换内容冲突；
create回包丢失后只按可由Surface Entry确定性重建的exact Ref Inspect。

## 3. neutral组装

`CompileModelToolsV1`输入必须是current且exact的
`ToolSurfaceManifestCurrentProjectionV1`、Material Reader与fresh clock。编译器按
Surface已冻结顺序逐项读取Material，并输出Model Invoker公开
`modelinvoker.Tool{Name, Description, Parameters, Strict}`。

V1只接受`Dialect=praxis.model/function-calling-v1`。它表示复用Model Invoker现有
neutral function-tool输入，不代表某一厂商。任何其他Dialect、hidden条目、
`Allowed=false`条目、Material NotFound/Unavailable、Schema或Description漂移、
Surface过期和clock rollback都Fail Closed，零输出。

`Strict=true`只是Schema表达要求，不改变Tool执行语义。V1现在把
`praxis.model/function-calling-v1`精确定义为portable expression profile：

- 名称必须匹配`[A-Za-z_][A-Za-z0-9_-]{0,63}`，即live OpenAI/Anthropic/Gemini
  adapter名称闭集的交集；
- 根Schema必须显式为object；每个object必须声明`properties`、
  `additionalProperties=false`，并把每个property精确列入`required`；
- Schema keyword只接受当前三类adapter均可表达的V1闭集；`allOf`、任意
  `x-vendor`字段或未知keyword Fail Closed；
- 该profile只保证表达可移植，不证明某条Route、Model、账户或Provider当前可用，
  更不授予Tool执行权。

输出顺序必须等于Surface顺序，输出摘要必须绑定Surface exact Ref、Dialect与完整
neutral Tools。portable profile的规则由Tool Owner版本化，不能跟随某个厂商SDK升级
而静默变化；需要厂商专用名称或Schema时必须使用未来独立Dialect/Profile，不能放宽V1。

## 4. Owner与依赖

- Tool Owner：Material合同、唯一Repository、neutral组装与输出摘要；
- Model Invoker Owner：公开`modelinvoker.Tool`及各provider协议映射；
- Harness/Application：未来composition root只注入Reader/编译结果，不创建Tool事实；
- Runtime/Review/Sandbox：不参与本地物化；实际调用仍走统一Action治理链。

允许依赖仅为Tool自身contract、Runtime public core/ports和Model Invoker根包公开
类型；禁止`model-invoker/internal`、厂商SDK、Harness实现和raw provider类型。

## 5. 验收与反例

- 同Ref 64并发Ensure只有一个winner；不同Ref可并行；race无数据竞争；
- nil/typed-nil Reader、nil/canceled context、NotFound、同ID换内容Fail Closed；
- Schema非object、尾随JSON、oversize、Schema digest错、Description digest错拒绝；
- Surface过期、clock rollback、Dialect漂移、顺序漂移、hidden/not-allowed条目零输出；
- 名称首字符/标点/长度不在portable闭集、Schema缺`required`、
  `additionalProperties`不为false、未知或厂商专用keyword均零输出；
- 编译返回后修改Schema bytes不影响Repository或下一次Inspect；
- import检查证明无厂商SDK、Model internal、Harness/Application实现依赖。

本切片只完成owner-local表达组装，不代表production Tool注入、G6B、真实Provider
执行或MCP Connect已启用。

## 6. 实现状态

`implementation_software_test_yes`：contract、唯一内存Repository、surface compiler、
portable function-tool profile与SDK exact入口已落盘。输出已由Model Invoker公开
`Request.Validate()`验证；portable名称/strict Schema反例进入Tool门禁。该状态不升级
任何production能力。

## 7. Model Route compatibility公共缺口

live `modelinvoker.Request.Validate()`只验证neutral结构；`CapabilityContract`只声明粗粒度
Tool Calling能力；`RouteInvoker.Resolve`不公开各provider dialect的名称/Schema/strict/
parallel离线映射结果。OpenAI、Anthropic、Gemini的实际`ValidateRequest/mapTools`仍由
Model Invoker Owner私有实现拥有，Tool不得导入或复制。

因此production多厂商“可调用”声明仍需要Model Owner additive只读Port：按RouteID和
exact Tool Surface/Compiled Tools摘要离线返回版本化Compatibility Projection，并在
Prepared Model Invocation进入Provider前复读current。Unavailable、unknown、route/profile
漂移或任一tool rejected时Provider调用必须为0。该Delta不阻塞Tool owner-local portable
编译，但阻止把portable编译成功升级为某条Route的production兼容事实。
