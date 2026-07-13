# 执行并集语义 Runtime v1 模块说明

## 1. 模块是什么

执行并集语义 Runtime v1 是 `model-invoker` 上方的一层稳定执行协议。它不要求 GPT、Claude、Gemini、Kimi、Qwen 或其官方 Harness 使用同一种工具；它统一的是：

1. 上层表达的执行意图；
2. 编译后选择的真实执行机制；
3. Runtime 从真实状态中观察到的效果；
4. 全程事件、审批、取消、验证和终态；
5. Direct API、App Server、Agent SDK、CLI 和 ACP 的可审计差异。

因此，上层可以统一表达“修改文件”，GPT 仍可走 `apply_patch`，Claude 仍可走 `Edit`，Gemini 可走 `replace`，Kimi/Qwen 可走各自 Harness 工具。Runtime 不把这些工具伪装成同一种工具，只把它们编译为同一个 Intent，并最终投影为可验证的 `FileChanged` 类 Effect。

本模块当前位于：

- 类型协议：`ExecutionRuntime/model-invoker/union/`
- Profile 编译：`ExecutionRuntime/model-invoker/profile/`
- 统一执行内核：`ExecutionRuntime/model-invoker/execution/`
- Direct 路由桥：`ExecutionRuntime/model-invoker/execution/direct/`
- Harness 路由桥：`ExecutionRuntime/model-invoker/execution/harness/`
- 真实效果观察：`ExecutionRuntime/model-invoker/effect/`
- 独立测试树：`ExecutionRuntime/model-invoker/tests/`

当前语义版本是 `praxis.execution-union/v1`。

## 2. 模块不是什么

本模块不是：

- 把所有模型缩减为公共能力交集；
- 把 Claude 的 `Edit` 强塞给 GPT，或把 GPT 的 `apply_patch` 强塞给 Claude；
- 把 Harness 的“任务完成”文字直接当作成功；
- 从用户机器自动寻找 CLI、继承登录环境或读取真实凭据；
- 用第三方程序接管官方 OAuth，或把订阅凭据转成裸 API 凭据；
- 在没有真实状态证据时合成文件、进程或 Computer Use Effect；
- 声称离线协议测试等价于真实 API、真实订阅或生产验证。

## 3. 五个顶层原语

上层与 Runtime 之间只需要理解五个顶层原语。

| 原语 | 作用 | 核心内容 |
|---|---|---|
| `UnifiedExecutionRequest` | 表达“想做什么” | 输入、指令、上下文引用、工具、输出合同、推理意图、会话意图、执行策略、预算、降级策略和 IntentGraph |
| `PreparedExecutionPlan` | 固化“准备怎样做” | 精确 Profile/Route、IntentGraph、候选 Mechanism、Expected Manifest、MappingReport、Residual、Route fingerprint 和摘要 |
| `UnifiedExecutionEvent` | 记录“正在发生什么” | Lifecycle、Intent、Mechanism、Model、Item、Effect、Control、Diagnostic 八类事件族 |
| `ExecutionCommand` | 控制“接下来允许怎样做” | 审批、拒绝、输入、取消、中断、继续和提供 Tool Result；包含幂等键及期望状态 |
| `UnifiedExecutionResult` | 汇总“最终真实发生了什么” | 终态、意图满足度、Mechanism trace、Effect、Verification、最终内容、Action、WorkspaceChange、Usage、Manifest、Residual 和摘要 |

五个原语的关系是：

```text
UnifiedExecutionRequest
        │ Profile Compiler
        ▼
PreparedExecutionPlan
        │ Adapter + Runtime
        ├──────── ExecutionCommand
        ▼
UnifiedExecutionEvent × N
        │ EventLedger + Reconcile + Verify + Project
        ▼
UnifiedExecutionResult
```

`PreparedExecutionPlan` 是执行前的确定性合同；`UnifiedExecutionEvent` 是事实账本；`UnifiedExecutionResult` 只能由完整事件账本投影，不能由上游直接返回。

## 4. Intent、Mechanism、Effect 三层协议

### 4.1 Intent：上层意图

Intent 只描述目标和约束，不指定厂商工具。首批 Intent 包括：

- 文件：`create_file`、`modify_file`、`rewrite_file`、`delete_file`、`move_file`、`create_directory`、`delete_directory`；
- 结构化输出：`produce_structured_output`；
- 工具：`call_tool`；
- 代码执行：`execute_code`；
- 计算机操作：`computer_use`。

每个 `IntentNode` 都有稳定 ID、目标、前后置条件、依赖、原子组、是否必需、幂等策略、冲突策略和可接受保真度。多个 Intent 组成有向依赖图 `IntentGraph`。

### 4.2 Mechanism：实际执行方式

`MechanismPlan` 是 Profile Compiler 对一个 Intent 生成的候选执行方式。它明确：

- 机制种类和能力引用；
- 能力来源；
- 执行所有者；
- 选择权属于 Runtime、模型、Harness 还是 Provider；
- 首选排序、硬约束、预期 Effect、验证计划和 fallback；
- 语义保真度。

真正运行时会生成 `MechanismAttempt`。Attempt 记录实际机制、原生工具身份、开始/结束时间、状态、清洗后的输入、输出引用、失败分类和副作用状态。计划与尝试必须分开：计划表达“允许怎样做”，尝试表达“实际怎样做了”。

能力来源固定区分为：

| 来源 | 含义 |
|---|---|
| `native` | 模型/协议原生能力 |
| `provider_hosted` | Provider 托管执行 |
| `harness_hosted` | Codex、Claude Code、Gemini CLI 等 Harness 执行 |
| `caller_hosted` | Praxis/调用方执行 |
| `emulated` | Praxis 受控模拟、校验或修复 |
| `unavailable` | 当前 Route 不可提供 |

保真度固定区分为 `exact`、`transformed`、`degraded` 和 `unavailable`。任何 transformed/degraded 路径都必须进入 MappingReport 或 Residual，不能静默当作 exact。

### 4.3 Effect：真实效果

Effect 不是模型的声明，也不是 Harness 的 provisional diff。`EffectRecord` 必须由 Praxis 观察器或验证器根据真实证据产生，并关联：

- 一个或多个 Intent；
- 一个真实 MechanismAttempt；
- 观测来源、证据引用、验证状态和发生时间；
- 被其取代的旧 Effect；
- 文件、结构化输出、工具调用、代码执行或 Computer Use 的专用载荷。

`VerificationRecord` 再把 Effect 与预期后置条件进行比对，状态可为 verified、partially verified、unverified、contradicted 或 not applicable。

模型文字、Harness 事件和调用方合成结果都可以作为 Event 或 Evidence，但不能越权创建 Effect。合成的 Tool Result 也不能产生 Effect。

## 5. Profile 是语义编译与执行策略

最终生效策略由三部分组成：

```text
EffectiveProfile = ModelBehaviorProfile
                 × HarnessCapabilityProfile
                 × RuntimePolicy
```

### 5.1 ModelBehaviorProfile

描述具体模型族和精确模型 ID 的后训练习惯：

- 哪些 Mechanism 更符合模型工具使用偏好；
- 每个偏好的排序；
- 已知失败模式；
- 官方行为证据、归因、有效期与摘要；
- 基准证据摘要。

它解决的是“同一个 Intent，这个模型最擅长怎样做”，而不是声称模型能力永远不变。

### 5.2 HarnessCapabilityProfile

描述当前调用面真正暴露了什么：

- Direct API、App Server、Agent SDK、官方 CLI 或官方 SDK；
- Harness 组件、版本、绝对可执行路径、二进制摘要和协议摘要；
- Expected Injection Manifest；
- 已注册、模型可见、可执行的 Mechanism；
- Harness 托管能力、原生特性、禁止特性和互斥特性；
- 审批、取消、steer、Tool Result、会话恢复等控制能力；
- 不透明字段及探测证据。

它解决的是“这条 Route 实际能怎样做”，并显式保留 Harness 注入造成的不纯净上下文。

### 5.3 RuntimePolicy

描述本次执行允许什么：

- Route/Provider/Offering/Model 身份锁；
- Intent 与 Mechanism allow/deny；
- 文件读写根、删除/移动、符号链接、原子写、大小限制；
- 进程 argv、cwd、shell meta、超时和网络；
- 网络 allowlist/denylist；
- Computer Use、不可逆操作和外部状态证据；
- 审批强度、过期、Action revision 变化；
- Secret 引用、明文禁止与脱敏；
- 重试/fallback、重新协调、副作用门禁；
- 最大时长、Action、并发和验证强度。

多层 RuntimePolicy 的合成遵循收紧原则：允许集合取交集，禁止集合取并集，预算取更严格值，验证与审批要求只增强不减弱。

## 6. Profile Compiler、Manifest 和 Residual

### 6.1 编译流程

Compiler 按以下顺序工作：

1. 校验 Request 和语义版本；
2. 用完整 `ProfileSelectionKey` 解析唯一 Profile；
3. 固化精确 Route、Provider、Model revision、Deployment、Region、Endpoint、Protocol、Offering、Auth route、Execution surface 和 Harness stack；
4. 合成 RuntimePolicy，并校验身份锁、预算和原生能力要求；
5. 对比 Expected/Actual Injection Manifest；
6. 规范化 IntentGraph；
7. 先按硬约束过滤 Mechanism，再按模型亲和度、保真度、Effect 可观测性、验证强度、确定性、成本、不透明增量和风险评分；
8. 为每个 Intent 生成主 Mechanism 与 fallback；
9. 生成 MappingReport、Residual、Route fingerprint 和确定性 Plan digest。

模型切换不是普通 fallback。自动模型切换会改变 Profile 与 Route fingerprint，因此当前 Compiler 明确拒绝“在同一个已准备计划内自动换模型”。

Mechanism fallback只能引用同一个Intent下已存在的Plan，且整个fallback图必须无重复、无自环、无任意环；这条约束同时保护手工Plan与Compiler产物。

### 6.2 两种 Manifest

模块有两个互补 Manifest：

- `InjectionManifest`：Profile 编译时使用，逐字段记录 present/absent/opaque、值、证据来源与置信度；
- `ContextManifestSummary`：执行时使用，记录实际组件、工具面、不透明边界和摘要。

Manifest 不是装饰信息，而是执行合同。Profile 层把漂移分为 P0-P3：身份、认证、沙箱、Secret、Workspace root 和执行所有者等 P0 漂移不可接受；允许的较低级漂移也必须成为 Residual。Runtime Preflight 再要求实际 Context Manifest 至少保留计划中的组件、工具和 opaque boundary，不能删改计划表面。

### 6.3 MappingReport 与 Residual

MappingReport 逐路径说明：

- 从哪个统一语义字段映射到哪个原生字段；
- 是 exact、configured、transformed、synthesized、degraded、retained extension、rejected 还是 unobservable；
- 能力来源、证据和具体原因。

Residual 是无法消除但允许继续的语义差异，必须说明路径、能力、种类、严重度、影响和缓解方式。Residual 会从 Profile 编译、Preflight、Harness、协调和验证一路汇总到最终 Result。

## 7. Runtime、EventLedger 与终态所有权

### 7.1 Adapter 生命周期

每个 Adapter 只实现三个步骤：

1. `Describe`：声明 Adapter 身份、事件来源和支持的 ExecutionKind；
2. `Preflight`：在发送用户 Prompt 前验证 Route、映射和实际 Manifest；
3. `Open`：只使用经过 Preflight 的同一 Invocation 打开 Session。

Session 只负责接收候选 Event、接收 Command 和关闭。需要在 Preflight 保留探测进程的 Harness 必须实现幂等清理；Request 或 Plan 在 Preflight 后发生变化时必须拒绝 Open。

`execution.NewInvocation` 是 Request/PreparedPlan 的封装边界：它固定 `request_digest` 与 Plan digest，并校验 IntentGraph、精确 Profile、ExpectedProfile 和 ExpectedRoute。封装后的 Request 或 Plan 任一字段被修改，Runtime 和 Adapter 都会在 Preflight/上游接触前拒绝。

### 7.2 EventLedger

`EventLedger` 是唯一事实账本，保证：

- EventID 唯一；
- 全局 Sequence 严格递增；
- Runtime 时间不倒退；
- 每条 Event 只有一个事件族载荷；
- Intent、Plan、Attempt、Item、Effect、Verification 的引用关系有效；
- Attempt/Item 状态只能合法前进；
- 终态唯一，终态后不能追加事件；
- 合成执行不能生成 Effect；
- 只有 `origin=praxis` 的事件可以提交 Effect、Verification 和统一终态。

Runtime和Projector使用`NewEventLedgerForPlan`/`ReplayForPlan`将账本绑定到密封的`PreparedExecutionPlan`。因此即便受信Adapter实现错误，也不能在事件流中新增、替换或接受计划外Intent/MechanismPlan；普通无Plan账本只保留给独立协议测试和通用事件工具。

Adapter 提供的 EventID、sequence、timestamp 会保存在 NativeIdentity、SourceSequence 和 SourceTimestamp 中；Runtime 重写全局身份、顺序、时间、Profile、Route 和 IngestedAt。这样既保留上游事实，又不允许上游占用全局审计权。

### 7.3 审批

审批绑定 `ApprovalID + ActionID + MechanismAttemptID + InputDigest + ActionRevision`，并带过期时间。Action 输入或 revision 变化会使旧审批失效；过期审批、旧摘要或旧 revision 不能复用。Command 使用幂等键和摘要，完全重复的命令可安全重放，内容不同却复用同一幂等键会被拒绝。

### 7.4 取消

取消不是“发出 cancel 就算完成”，而是状态链：

```text
requested → dispatched → acknowledged → quiesced
                                  │
                                  ▼
                             reconciling → reconciled
```

只有取消已确认、进程/后台工作已静止并完成副作用协调，Runtime 才能输出 `cancelled`。如果是否静止或是否产生副作用无法确认，终态必须是 `indeterminate`。

### 7.5 统一终态

Provider、CLI、SDK 或 App Server 的 completed/end_turn 只会成为 `route_terminal_candidate`。Runtime 随后执行：

1. 等待后台工作归零；
2. Reconcile 真实副作用；
3. 由 Praxis 提交 Effect；
4. Verify 必需后置条件；
5. Project 唯一统一终态和 Result。

只有所有必需 Intent 均被 verified，且无未知副作用、协调错误或验证错误，才能输出 `succeeded`。部分完成输出 `partial`；矛盾证据输出 `failed`；状态未知输出 `indeterminate`。

## 8. Direct 与 Harness 路由边界

| 路由 | 当前实现边界 | 上下文与工具处理 | 终态与 Effect |
|---|---|---|---|
| Direct API | `execution/direct` 复用现有 `routegateway`；精确 Route/Model/Invocation；支持非流和流、caller-hosted function tool continuation | Request 映射到现有统一 Provider 请求；ToolPolicy 只暴露显式允许的 caller tool，未知 ToolID、未声明工具调用和 Harness-owned 工具在上游前 fail closed；原生 Raw 只暴露摘要/大小/JSON 标记 | Provider 完成只生成候选事件；Effect、验证和统一终态仍由 Runtime 负责 |
| Codex App Server | `execution/harness/codexappserver`，显式本地 JSON-RPC v2；initialize 后复用同一进程，执行 thread/start、turn/start、审批和中断 | 固定 model/cwd/sandbox 或 permissions；dynamic tools 需要 experimental capability；App Server 的隐藏指令/工具作为 opaque residual | native item/diff/turn complete 均为候选；provisional diff 不等于 FileChanged |
| Claude Agent SDK/CLI | `execution/harness/claude` + `streamjson`；双向 stream-json；先 initialize 和 SystemMessage(init)，再发送 Prompt | 以 init 信息构建 Actual Manifest；保留 Claude Code 的原生工具习惯和 Harness 注入，不宣称纯净模型 | SDK result/end 只结束 Route；真实文件、工具和进程结果由 Praxis observer 重建 Effect |
| Qwen Code SDK | `execution/harness/qwen` + `streamjson`；双向 stream-json；Preflight 复用同一受控进程 | 用 SDKSystemMessage/init 验证工具面、会话与上下文；bare/core_tools 等互斥能力由 Profile 约束 | Harness result 不是统一成功；审批、取消、Effect 和终态走 Runtime |
| 通用 ACP | `execution/harness/acp`；显式 JSON-RPC NDJSON；initialize、session/new、prompt、updates 和 cancel | 标准 ACP 只提供协议桥；Agent 可能增加原生指令/工具，必须记录 opaque residual；当前离线切片只接受 text/JSON prompt，image/resource/resume/load-session 显式不可用 | ACP end_turn 是 Route 候选，必须经过 Reconcile/Verify |
| Gemini CLI | `execution/harness/gemini` 已在 ACP 上实现专属 wrapper；`implementation_status=implemented_offline`、`offline_contract_tests=passed` | 强制官方 CLI `--acp` 启动方式，要求显式确认 first-user session context，并把该不透明上下文写入 Actual Manifest/Residual | 专属 fake process、取消、Race、shuffle 和全仓门禁已通过；真实套餐`live_verification=not_run` |
| Kimi Code | `execution/harness/kimicode` 已在 ACP 上实现专属 wrapper；`implementation_status=implemented_offline`、`offline_contract_tests=passed` | 只接受 `current_acp` 和 `acp` 子命令；从启动参数、Session options、Request 与 Plan 中拒绝 legacy wire、agent_file、str_replace_file | 专属 fake process、取消、Race、shuffle 和全仓门禁已通过；真实会员`live_verification=not_run` |

Harness 共用的进程边界位于 `execution/harness/process`：

- 必须提供绝对可执行路径，不通过 PATH 自动发现；
- 可选固定 SHA-256，并回报实际路径、摘要、PID、退出和信号证据；
- 不经 shell，不继承父进程环境，拒绝敏感或动态加载环境变量；
- cwd 必须位于显式允许集合；
- stdout/stderr 和 JSONL/JSON-RPC frame 有硬上限；
- 取消先发 SIGTERM，再在有界时间后 SIGKILL，并验证进程组静止；
- Close 幂等。

这些限制只证明 Praxis 如何安全启动被明确指定的 Harness；它们不会寻找登录态，也不会代替官方认证组件。

## 9. Effect Observer

Effect Observer 位于 `effect/`，按真实效果类型分工。

### 9.1 文件

`FileObserver` 只观察允许的绝对根目录，使用真实 `Lstat`/`Stat`、类型、大小、mode、mtime、symlink 和内容 hash。它支持：

- create/modify/rewrite/delete；
- directory create/delete；
- move 的源和目标 before/after；
- 内容可捕获时生成脱敏 unified diff；
- before/after hash 和预期条件校验；
- 越界路径、危险 symlink、超大文件和无变化拒绝。

### 9.2 结构化输出

结构化输出使用严格 JSON 解码和 Draft 2020 JSON Schema 校验：

- 重复 key 拒绝；
- 外部 `$ref` 拒绝；
- Schema 本身无效时不进入修复循环；
- JSON/Schema 不合格可在明确次数内通过 Praxis repairer 修复；
- 只有最终严格通过的候选才生成 Effect；
- Effect 保留 native/provider-hosted/harness-hosted/emulated 来源与保真度。

### 9.3 Tool Call

Tool observer 只记录真实执行过的调用，绑定 ToolID、ActionID、owner、mechanism、result origin 和副作用状态；输入输出保存摘要，不把完整敏感载荷写入公共 Effect。提议、合成或仅由模型声称的结果不得进入该观察器。

### 9.4 代码执行

Process observer 校验真实 argv、Runtime identity、exit code、stdout/stderr 摘要、持续时间、环境指纹和可选网络证据。缺少 exit code 或必需网络证据会是 unverified；argv、Runtime 或退出码冲突会是 contradicted。

### 9.5 Computer Use

Computer observer 记录 action、target、before/after evidence 和外部 readback。不可逆操作没有审批时必须 contradicted；缺少要求的状态证据时只能 unverified，不能凭截图描述或模型文字宣告成功。

## 10. 测试布局

仓库规则要求所有 `_test.go` 只出现在独立 `tests/` 树。Runtime v1 的测试分层如下：

| 目录 | 测试性质 | 主要覆盖 |
|---|---|---|
| `tests/profilecompiler/` | 白盒/合同测试 | Profile 唯一解析、三因子策略合成、硬约束优先、Manifest P0-P3、MappingReport、Residual、确定性摘要 |
| `tests/executionunion/` | 白盒状态机测试 | Registry、EventLedger、事件权限、审批 revision/TTL/幂等、取消链、协调、终态和结果投影 |
| `tests/effectobserver/` | 黑盒本机状态测试 | 临时 Workspace 文件变化、严格 Schema、Tool Call、进程证据、Computer Use 规则 |
| `tests/executiondirect/` | 本机集成测试 | fake routegateway backend、非流/流、tool continuation、Harness tool 拒绝、Route/Model 固定 |
| `tests/harnesslocal/process/` | 本机黑盒测试 | 绝对进程、环境清洗、framing、输出上限、取消与进程组静止 |
| `tests/harnesslocal/streamjson/` | 协议测试 | 双向 JSONL、控制请求相关性、边界与关闭 |
| `tests/harnesslocal/codex/` | 本机 fake Harness 集成 | App Server initialize/thread/turn、映射、审批/取消、候选事件 |
| `tests/harnesslocal/claude/` | 本机 fake Harness 集成 | Agent SDK init、Manifest、stream-json 事件与控制 |
| `tests/harnesslocal/qwen/` | 本机 fake Harness 集成 | Qwen SDK init、Manifest、stream-json 事件与控制 |
| `tests/harnesslocal/acp/` | 本机 fake Harness 集成 | ACP initialize/session/prompt/update/cancel 和协议拒绝 |
| `tests/unioncontract/` | 类型白盒/模糊测试 | 五顶层原语、ID/因果关联、深拷贝、摘要、敏感字段、Fuzz 与基准 |
| `tests/conformance/` | 跨层负例与六路集成 | N01-N14；同一四类 IntentGraph 在 Direct/Codex/Claude/Gemini/Kimi/Qwen 上 Mechanism 不同而 Effect/Verification/Satisfaction 收敛 |
| `tests/performance/` | Fuzz 与基准 | Profile 编译、Manifest diff、Event replay、文件快照和 Harness frame |

测试必须使用 fake backend、fake process 和临时目录；默认验收不得发现系统 CLI、复用用户配置目录、读取 API Key/OAuth 或发出远端请求。

最终离线验收应至少包含：格式检查、`go mod verify`、`go vet`、普通测试、shuffle、race、定向重复、覆盖率、fuzz、benchmark、完整 integration-tag 离线套件（真实 smoke 默认 Skip）以及统一 `scripts/verify-offline.sh`。

### 10.1 2026-07-13 首轮实际离线验收（历史快照）

- `go test -count=1 ./...`、`go vet ./...`：通过；
- 新 Runtime 相关测试 `-shuffle=on -count=20`：通过；
- Qwen/Claude/Gemini/Kimi取消路径普通压力各 `500` 次、Race压力各 `50` 次：通过；四条Harness全套测试shuffle `20`轮、全仓shuffle `5`轮：通过；
- `go test -race -count=1 ./...`：通过；
- `-covermode=atomic -coverpkg=./...`：合并语句覆盖率 `76.4%`；
- Fuzz 3 秒窗口：Manifest diff `302,498` 次、Event replay `19,500` 次、Harness frame `147,040` 次、Union event `667,224` 次，合计`1,136,262`次，均通过；
- 基准三轮结果：Profile compile `0.508-0.520 ms/op`，Manifest diff `89.7-90.1 us/op`，256-event replay `3.02-3.16 ms/op`，文件快照 `114.2-115.2 us/op` / `746.6-753.0 MB/s`；
- 六 Route 本地集成、N01-N14、integration build-tag 仅编译和统一离线脚本：通过；
- 真实 API、OAuth、订阅与官方二进制联调：`not_run`，等待用户提供测试边界。

离线门禁曾捕获Qwen/Claude双向stream-json取消响应与本地pending发布之间的竞态。当前实现用发布屏障保证接收侧处理快速`control_response + result`前已经建立取消关联，再由Runtime按ack、quiescence、reconcile顺序决定终态；上述定向压力与全量门禁均在修复后重跑。

### 10.2 第二轮 Review 与测试加固

在真实 API/订阅联调前，Runtime v1 又完成了一轮独立的语义、执行与 Harness 三线审查。该轮不是只增加 happy-path 覆盖率，而是以反例证明并修复以下合同缺口：

- 语义/Profile/Effect：v1 tagged union 封闭、扩展命名空间和高置信凭据拒绝；文件根、Move 目标、symlink、Effect/Verification 双向关联、supersession、Computer readback、typed-nil repair 和无效 UTF-8 摘要；
- Runtime/Direct：Intent/Plan/Attempt/causation 交叉身份、Session/Turn spoofing、终态前 Close、Effect/Verification 投影、最终响应中首次出现的工具调用、并发重复 Tool Result 只续跑一次、工具名称与副作用状态篡改；
- Harness：Claude/Qwen 纯结构化输出 Attempt、Execution-scoped Attempt ID、并发会话隔离、默认安全 Skip 和增强的 live-smoke 明文凭据门禁；
- 跨 Route 集成：`tests/integration/harness_routes_offline_test.go` 直接实例化 Codex/Claude/Gemini/Kimi/Qwen 生产 Adapter，通过受控 fake child process 走完 `Preflight → Open/Runtime → Reconcile → Verify → Result`，并验证同一 Adapter 的双 Execution 隔离。

最终代码上的离线验收结果：

- `go test -count=1 ./...`、`go test -shuffle=on -count=5 ./...`、全仓 Race/Vet、integration-tag 五 Route 普通/shuffle/race、统一 `scripts/verify-offline.sh`：通过；
- 默认全仓 `-covermode=atomic -coverpkg=./...`：`76.6%`；加入 integration-tag profile 后合并语句覆盖率：`76.7%`；
- 五项 3 秒定向 Fuzz：Union Event `592,376`、Request Control `145,092`、Manifest Diff `306,053`、Event Replay `14,935`、Harness Frame `112,858`，合计 `1,171,314` 次，均通过；
- 三轮基准：Profile compile `1.340-1.541 ms/op`，Manifest diff `102.9-247.3 us/op`，256-event replay `3.068-3.337 ms/op`，文件快照 `118.2-161.0 us/op` / `534.33-727.69 MB/s`。

仍保留三个明确 P2 边界：凭据扫描只覆盖高置信秘密形态，不扫描普通用户正文的高熵片段；文件 Observer 的 authorize-then-use TOCTOU 需要未来 Sandbox/fd/openat 执行层闭合；新厂商 ContentPart 不能通过未版本化字段偷渡，必须升级合同。真实 API、OAuth、订阅账号和官方二进制依旧为 `not_run`。

## 11. 离线完成与真实联调边界

### 11.1 离线能够证明

- 类型、摘要、拷贝和校验合同稳定；
- Profile 能将统一 Intent 编译到特定 Route 的 Mechanism；
- Harness 注入和能力漂移会被 Manifest/Residual 捕获；
- Direct 和 Harness Adapter 不拥有 Effect 或统一终态；
- 审批、取消、事件顺序、副作用不确定性和终态规则可复现；
- 本地受控进程、framing 和 fake 协议能够完整走通；
- 文件、结构化输出、工具、进程和 Computer Use 的观察规则可由真实本机状态验证。

### 11.2 离线不能证明

- 真实 API Key、云账号、ChatGPT/Claude/Gemini/Kimi/Qwen 订阅当前可用；
- 官方 CLI/SDK 的最新版本与 fake 协议完全一致；
- 账号套餐、配额、地域、模型 entitlement 和条款没有变化；
- 模型在真实任务上的工具偏好与 Profile 证据永远一致；
- 真实 Harness 没有新增隐藏指令、工具、Hook 或后台工作。

### 11.3 后续真实联调必须做什么

用户提供真实 API 或官方订阅环境后，每条 Route 必须单独进行：

1. 使用`tests/integration/harness_routes_smoke_test.go`中的单Route测试入口，固定官方组件版本和二进制摘要；
2. 只通过官方认证路径登录，不提取或转交订阅 OAuth；
3. 先探测 Actual Manifest，再决定是否允许发送 Prompt；
4. 运行最小无副作用握手、结构化输出、Tool Call、文件变更、审批和取消用例；
5. 对真实 Workspace/外部状态做 Effect readback；
6. 对照离线 Event trace 检查 native event、终态和背景工作漂移；
7. 更新 Profile 证据、有效期、Manifest 和 Residual；
8. 只有该 Route 的真实验收通过后，才把状态从 `implemented_offline` 提升为 `live_verified`。

五条官方Harness入口均受`integration` build tag、全局`PRAXIS_LIVE_TESTS=1`、全局`PRAXIS_HARNESS_PROBE=confirmed`和单Route`..._HARNESS_LIVE=confirmed`共同保护。它们要求显式绝对executable、SHA-256、cwd、HOME、精确模型/版本/argv及Route专属init期望；子进程不接收API Key、OAuth token或Cookie。2026-07-13已用临时ChatGPT Pro登录完成Codex单Route真实验证；Claude、Gemini、Kimi、Qwen仍为`not_run`。

### 11.4 Codex真实联调增量

- 当前官方App Server使用versionless JSON-RPC-like NDJSON；Praxis以独立`codex_app_server_ndjson`方言兼容，ACP严格JSON-RPC 2.0未被放宽；
- Codex订阅Profile模型现场更新为`gpt-5.6-sol`；
- 代理变量必须显式命名后才能进入清洗环境，Manifest只公开名称和摘要；
- 原生`error`事件保留`codexErrorInfo`与`willRetry`，可重试流错误不再被错误提升为统一终态；
- WebSocket受代理限制时，用官方Codex HTTP-only provider选择HTTPS/SSE，认证仍由官方Codex读取临时登录；
- 登录/连接smoke只验证最小无副作用marker，`outputSchema`能力必须由独立conformance用例证明，避免把能力失败误判为订阅登录失败。

真实联调必须逐 Route 授权和执行。本说明中的“已实现”默认只表示源码与离线测试资产存在，不表示真实账号、真实模型或生产可用。

## 12. 维护原则

后续新增模型、Provider 或 Harness 时，不应增加新的顶层调用语义。应按以下顺序扩展：

1. 为新 Route 增加精确 `ProfileSelectionKey`；
2. 根据官方实现证据补充 ModelBehaviorProfile；
3. 用真实探测补充 HarnessCapabilityProfile 和 Expected Manifest；
4. 将现有 Intent 映射为该 Route 擅长的 Mechanism；
5. 对无法等价的部分生成 MappingReport/Residual；
6. 让 Effect Observer 继续从真实状态产生统一结果；
7. 增加 fake 协议、负例、race/fuzz 和后续真实联调用例。

只有当新能力无法由现有 Intent/Effect 正确表达时，才扩展并集类型；不能因为厂商新增了一个工具名字，就把厂商工具名提升为新的顶层原语。
