# 执行语义并集 Runtime 实施计划 v1

## 1. 状态与授权

- 模块：`model-invoker`
- 计划版本：`v1`
- 创建时间：2026-07-13
- 当前状态：陈旧计划（已完成）
- 实现位置：`ExecutionRuntime/model-invoker/`
- 授权依据：用户已在完成 Profile、Intent/Mechanism/Effect 和代表 Route 设计后，明确要求继续完成全部代码、白盒、黑盒和本地集成测试。
- 凭据边界：本轮不创建、读取或调用真实 API Key、OAuth、账号登录和订阅额度；真实联调由用户最后提供凭据后单独执行。
- 事实源：
  - [执行语义并集 v1](../../design/model-invoker/execution-semantic-union-v1-draft.md)
  - [Intent、Mechanism、Effect 与 Profile 路由](../../design/model-invoker/intent-mechanism-effect-profile-routing-v1-draft.md)
  - [代表 Route 纸面编译与一致性合同](../../design/model-invoker/representative-route-paper-compilation-and-union-conformance-v1.md)
  - [上游官方 Agent 行为与 HarnessDelta](../../design/model-invoker/upstream-official-agent-behavior-and-harness-delta-research-20260713.md)

## 2. 最终可预见产物

本计划完成后，现有 `model-invoker` Go module 将新增一套可离线运行的执行语义层：

1. `union`：稳定的 Request、PreparedPlan、Event、Command、Result，以及 Intent、Mechanism、Effect、Manifest、Residual 类型和校验；
2. `profile`：完整 SelectionKey、三类 Profile、作用域合并、Manifest 比较、RouteFingerprint 和确定性 Compiler；
3. `effect`：文件真实状态观测、hash/diff、结构化输出校验、进程证据与 Intent Satisfaction 验证；
4. `execution`：追加写事件账本、审批/取消状态机、终态投影、Adapter 注册与统一 Orchestrator；
5. `execution/direct`：桥接现有 `RouteGateway/modelinvoker`，保留 caller-hosted tool loop 和原生 Responses 等模型事件；
6. `execution/harness`：安全进程 transport、JSONL/JSON-RPC/ACP framing，以及 Codex、Claude、Gemini、current Kimi Code、Qwen 的官方 Harness 合同适配；
7. fake HTTP、fake process、临时工作区和六 Route conformance 测试基座；
8. 用户无需先读源码即可理解和验证的 `.properties.rax/module/model-invoker` 中文说明、memory 和总体索引。

所有 Harness 路线在没有真实二进制、SDK、登录态或凭据时保持 `implemented_offline`，不得标成 `live_verified`。

## 3. 物理边界

### 3.1 保留的既有边界

- 不扩展 `modelinvoker.Provider` 为 Agent/Harness 接口；现有 Direct Provider、协议 driver 和 Factory 保持兼容。
- 不把 Codex、Claude、ACP 或 CLI 注册成现有 Provider/Factory。
- 不修改既有 `upstream.RouteIdentity` 七维身份；新的 execution surface 与 harness stack 进入独立 `profile.SelectionKey`。
- 既有 20 项 Model Capability 继续表达模型协议能力；Origin、Owner、Event Fidelity 和 Effect Observability 进入并集/Profile 合同。
- `routegateway.Gateway` 继续负责 Direct Route 的 Catalog、凭据、Factory 和生命周期，并作为 Direct bridge 的下游。

### 3.2 新增包

```text
ExecutionRuntime/model-invoker/
|-- union/                         # 纯语义类型、校验、规范化和 digest
|-- profile/                       # Profile registry/compiler/manifest/fingerprint
|-- effect/                        # observer/verifier
`-- execution/
    |-- direct/                    # 现有 model-invoker/routegateway bridge
    `-- harness/
        |-- process/               # 无 shell 的受控子进程与 framing
        |-- codexappserver/        # JSON-RPC app-server v2
        |-- claude/                # Agent SDK/official CLI stream-json bridge contract
        |-- gemini/                # 官方 CLI ACP专属wrapper
        |-- kimicode/              # current Kimi Code ACP；拒绝legacy Wire
        `-- qwen/                  # SDK/CLI stream-json，bare-fixed/nonbare 分离
```

Go 继续拥有语义、策略、事件、Effect、Verifier 和进程监督。只有官方 SDK 无法通过稳定 CLI/App Server/ACP 进程合同表达时才允许新增 TypeScript glue；本切片先以可注入 executable/argv/env/protocol 合同实现，不增加新的顶级 module，也不自动发现本机真实 CLI。

## 4. 核心合同

### 4.1 五个顶层原语

- `UnifiedExecutionRequest`
- `PreparedExecutionPlan`
- `UnifiedExecutionEvent`
- `ExecutionCommand`
- `UnifiedExecutionResult`

### 4.2 三层因果链

```text
IntentGraph
  -> MechanismPlan / MechanismAttempt
  -> observed Effect / Verification
  -> IntentSatisfaction
  -> unique Execution terminal
```

Harness/Provider 终态只形成 `route_terminal_candidate`。取消 quiescence、后台 drain、副作用 reconcile 和 required verification 完成前不得发统一终态。

### 4.3 事件不变量

- family 固定为 `lifecycle|intent|mechanism|model|item|effect|control|diagnostic`；
- 全局 sequence 从 1 严格递增，保留 source sequence/timestamp；
- tool call、approval、execution item、tool result、effect 分开；
- synthetic tool result 必须 `executed=false`，不能生成 Effect；
- 终态唯一，终态后拒绝任何事件；
- reasoning 只发布允许公开的 summary/provider-exposed 内容，不采集隐藏思维链。

## 5. 实施阶段

### 阶段 A：类型与确定性基础

- [x] 定义全部 ID、枚举、Tagged Union、Request/Plan/Event/Command/Result；
- [x] 实现深拷贝、规范化 JSON、稳定 SHA-256 digest 与公共校验错误；
- [x] 实现 IntentGraph 拓扑校验、机制/Effect/Verification 关联校验；
- [x] 实现 secret-safe 字段边界和公开摘要。

### 阶段 B：Profile Compiler

- [x] 定义 `ProfileSelectionKey` 与 harness component stack；
- [x] 定义 `ModelBehaviorProfile`、`HarnessCapabilityProfile`、`RuntimePolicy`；
- [x] 实现 organization/user/workspace/task 约束合并；
- [x] 实现 ToolSurfaceManifest、Expected/Actual Manifest、证据质量和 P0-P3 漂移；
- [x] 实现唯一 Profile 解析、hard filter、确定性评分、primary/fallback 选择；
- [x] 实现 PreparedPlan、MappingReport v2、Residual 和 RouteFingerprint。

### 阶段 C：首批原语与 Effect

- [x] 文件 Create/Modify/Rewrite/Delete/Move/Directory 意图、机制和 Effect；
- [x] 安全路径解析、symlink 栅栏、before/after snapshot、hash、metadata 与 unified diff；
- [x] structured output 的 native/harness/tool/json-object/emulated/prompted 分类；
- [x] JSON parse/schema validation、repair attempt 记录与最终 digest；
- [x] Tool manifest、参数 revision 审批、result origin 和真实执行关联；
- [x] provider/harness/caller hosted code execution 证据；
- [x] Computer Use 能力、不可逆审批和证据不足时的 fail-closed/unverified。

### 阶段 D：执行、取消与投影

- [x] 追加写 EventLedger 与确定性 replay；
- [x] Item、Approval、Attempt、Cancel、Reconcile 和 Terminal 状态机；
- [x] Command dispatcher、pending approval 失效和幂等；
- [x] Adapter registry、Session 和 EventSink 合同；
- [x] Orchestrator 的 prepare → policy gate → execute → reconcile → verify → project；
- [x] Result projector 保留部分副作用、route error、verification 和 residual。

### 阶段 E：Direct 与 Harness Adapter

- [x] Direct bridge 映射 text/tool/structured/reasoning/session，caller-hosted 执行不重复归属；
- [x] 受控 process transport：显式 executable、无 shell、env allowlist、cwd、stdout/stderr limit、context cancel、进程组回收；
- [x] JSONL、JSON-RPC、ACP framing：frame limit、非法 UTF-8、半帧、未知事件、ID correlation 和 EOF 终态；
- [x] Codex app-server：initialize/thread/turn/item/diff/approval/interrupt；
- [x] Claude：init/partial/tool/result/permission/interrupt 与 strict output；
- [x] Gemini：first-user session context、ACP tool/permission/cancel；不把未固定的headless方言混入同一Route；
- [x] current Kimi Code：current ACP，拒绝 legacy Wire/agent_file/str_replace_file，end_turn 不冒充成功；
- [x] Qwen：bare-fixed 与 controlled-nonbare，拒绝 `--bare + coreTools`；
- [x] 每条 Route 的 Expected Manifest、事件映射、terminal candidate 和 fail-closed。

### 阶段 F：测试、说明与同步

- [x] 白盒：Profile、Manifest、Fingerprint、状态机、Effect、Verifier、Projector；
- [x] 黑盒：fake HTTP/process、stdin/stdout/stderr/exit/signal、故障注入；
- [x] local integration：六 Route 同一 IntentGraph，机制不同、Effect 与 Satisfaction 一致；
- [x] N01-N14 negative golden 全部机器化；
- [x] fuzz、race、shuffle、coverage、benchmark 和统一离线脚本；
- [x] 更新 module 说明、properties 索引、plan 状态与 memory。

## 6. 测试布局

```text
tests/
|-- unioncontract/
|-- executionunion/
|-- profilecompiler/
|-- effectobserver/
|-- executiondirect/
|-- harnesslocal/{process,streamjson,codex,acp,claude,gemini,kimi,qwen}/
|-- conformance/              # N01-N14 + semantic_union_routes_test.go
`-- performance/
```

普通 local loopback/process 测试不使用现有 `integration` tag；该 tag 继续只表示需要真实凭据的烟测。Harness fake 必须显式指定测试进程，禁止通过 PATH 自动发现用户本机真实 CLI。

最终实现沿用仓库既有的外部测试包与各测试文件内fake，没有新增原计划中的`internal/testkit/unioncontract`和`internal/testkit/harnessprocess`；这两项不是公共Runtime依赖，删除后没有减少测试覆盖面。

## 7. 白盒、黑盒和集成验收

### 7.1 白盒

- Profile 零匹配/多匹配在上游前拒绝；
- hard constraint 先于评分，相同输入与 digest 产生相同 Plan；
- P0/P1 漂移 fail closed，opaque 不等于 absent；
- approval 绑定 action revision/input digest，变更后失效；
- cancel requested 不等于 quiesced，未知副作用时为 indeterminate；
- CompletionClaim 不生成 Verified Effect；
- Result 可从同一 EventLedger 确定性重放。

### 7.2 黑盒

- frame 成功、拒绝、超时、取消、半帧、超大帧、非法 UTF-8、未知版本；
- argv/env/cwd 捕获与脱敏；
- stdout/stderr、exit code、SIGTERM/SIGKILL 和进程树回收；
- Codex/Claude/Gemini/Kimi/Qwen 的 native 事件到统一事件映射；
- HTTP Direct tool loop 与断流后 Effect reconcile。

### 7.3 本地集成

- 六条代表 Route 使用同一 IntentGraph：ModifyFile、CreateFile、ExecuteCode、ProduceStructuredOutput；
- MechanismTrace 允许不同，最终文件 hash/diff、进程证据、structured validation 和 Satisfaction 必须一致；
- route terminal、transport error 与真实 Effect 分开；
- 订阅与 PAYG、API 与 Harness 之间不串 Route、凭据、状态或工具 owner。

## 8. 验证命令

在 `ExecutionRuntime/model-invoker/` 执行：

```bash
bash ./scripts/verify-offline.sh
go test -count=1 ./tests/executionunion ./tests/profilecompiler ./tests/effectobserver ./tests/harnesslocal/... ./tests/conformance
go test -shuffle=on -count=20 ./tests/executionunion ./tests/profilecompiler ./tests/effectobserver ./tests/harnesslocal/... ./tests/conformance
go test -race -count=1 ./tests/executionunion ./tests/profilecompiler ./tests/effectobserver ./tests/harnesslocal/... ./tests/conformance
go test -covermode=atomic -coverpkg=./... -coverprofile=/tmp/model-invoker-union.cover ./...
go tool cover -func=/tmp/model-invoker-union.cover
go test -run '^$' -bench 'Benchmark(ProfileCompile|ManifestDiff|EventReplay|FileSnapshot)' -benchmem ./tests/...
```

选定 fuzz target 以固定短时窗口实际执行；完整 fuzz campaign 留给后续 CI/nightly，不把不稳定耗时塞进当前 30 分钟主 job。

## 9. 明确禁止

- 不读取、创建、复用或打印真实凭据；
- 不登录官方账号，不消耗订阅和付费额度；
- 不反代或自接管厂商消费者 OAuth；
- 不将 Harness 伪装为干净 Direct API；
- 不把模型文字、provisional diff、end_turn 或 Harness Result 单独当作 Effect；
- 不在副作用未知时自动 fallback；
- 不把 emulated/prompted 结构化输出标成 native；
- 不自动发现真实 CLI，不在默认测试中读取用户配置目录和登录态。

## 10. 完成条件

只有以下条件同时成立，本计划才标记为“陈旧计划（已完成）”：

1. 阶段 A-F 全部落地，公共类型与现有 Direct API 兼容；
2. 六条代表 Route 都有可运行的离线 Adapter 合同和 fake fixture；
3. N01-N14、白盒、黑盒、本地集成、shuffle、race、vet 和离线总门禁通过；
4. coverage 与 benchmark 已实际生成并记录，不伪造阈值结论；
5. module 说明、properties 索引和 memory 已同步；
6. 真实账号/API/订阅项目明确保留为 `not_run`，等待用户提供凭据。

## 11. 完成记录（2026-07-13）

- 阶段 A-F 全部完成；`union/profile/effect/execution`、Direct 与 Codex/Claude/Gemini/Kimi/Qwen Harness Adapter均已落地；
- 同一 ModifyFile/CallTool/ExecuteCode/ProduceStructured IntentGraph 已在六个代表Profile上完成真实Compiler与本地Runtime收敛测试；
- 普通测试、20次shuffle、全仓Race/Vet、N01-N14、integration仅编译和统一离线脚本通过；
- Qwen/Claude/Gemini/Kimi取消路径各500次普通压力、各50次Race压力，以及Harness 20轮和全仓5轮shuffle在最终修复后通过；
- EventLedger最终绑定密封PreparedPlan权限面，Mechanism fallback图的存在性、同Intent和无环约束已补齐并由恶意Adapter/手工Plan负例覆盖；
- Codex/Claude/Gemini/current Kimi/Qwen五路真实Harness smoke入口已受build tag、三重确认、二进制SHA与精确Manifest门禁保护并完成离线编译；真实进程与账号仍为`not_run`；
- 合并语句覆盖率为`76.4%`；最终四个3秒Fuzz窗口合计执行`1,136,262`次；四项性能基准已在最终代码上重跑三轮；
- Gemini/Kimi的`implementation_status=implemented_offline`、`offline_contract_tests=passed`；真实API、OAuth、订阅和官方二进制的`live_verification=not_run`；
- 详细结果见[执行语义并集Runtime离线实现与验收完成快照](../../memory/model-invoker/20260713-041100-执行语义并集Runtime离线实现与验收完成.md)。
