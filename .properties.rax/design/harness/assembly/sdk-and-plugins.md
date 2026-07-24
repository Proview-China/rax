# Harness Assembly SDK、插件与外部边界

## 1. SDK职责

Assembly SDK是对版本化公共对象的构建、编译和说明入口，不是另一个Runtime SDK或Plugin特权通道。

计划中的Go公共能力仅包括：

- `BuildInput`：以类型化Builder收集已获准对象和稳定Ref；
- `Compile`：规范化、验证DAG、解析Slot/Phase贡献并生成候选Graph；
- `Inspect`：只读查看Generation、Manifest、Graph、Diagnostic与Residual；
- `Explain`：给出某Slot/Phase/拒绝结论的完整来源链；
- `Diff`：比较两个Generation的规范化语义差异；
- `Conformance`：校验Expected/Actual、Owner、Schema、Effect、Run Requirement和currentness。

SDK不提供Run、Pause、Resume、Cancel、Permit、Tool执行、Sandbox操作、领域Commit或Kernel内部访问。运行控制继续由Runtime/Application公共SDK承担。

## 2. 组件贡献边界

组件通过Harness定义的namespaced、versioned公共对象声明：

```text
ModuleDescriptorV1
+ SlotContributionV1 -> existing SlotSpecV1
+ PhaseContributionV1 -> existing HookFaceSpecV1/PhaseID
+ PortSpecV1/PortBindingV1
+ DependencySpecV1
+ Run Requirement/Settlement refs
```

组件不得：

- 新增公共Slot/HookFace/Phase枚举或修改其合并规则；
- 导入Harness kernel/fakes/internal或私有`ContextPort`/`ModelTurnPort`/`EventCandidatePort`；
- 通过通用Hook任意改Context、联网、写Fact或发起未治理Effect；
- 把Provider Observation、Model Tool Call、Phase Receipt提升为正式领域结论。

公共Catalog由Harness Assembly Owner版本化发布。未知公共ID、主版本不兼容或Contribution超出HookFace权限上限时Fail Closed。

## 3. Plugin模型

Plugin是可替换实现的交付单位，不是权威等级。其生命周期为：

```text
discovered -> described -> validated -> compiled
-> runtime-activated currentness check -> instantiated -> observed/settled
```

- Manifest保存`ModuleFactoryDescriptorV1`，不保存原始函数指针、Secret或厂商SDK对象；
- trusted in-process Go实现也必须经过Artifact Digest、Manifest、Binding和Capability校验；panic隔离为结构化失败；
- remote/WASM/out-of-process仅作为未来ConstructionMode，不在V1预选技术、RPC或进程拓扑；
- 实例化、握手、资源申请、联网或Cleanup若产生Effect，必须沿Operation V3治理链；
- Plugin卸载不等于资源已清理；以领域Owner Inspect/Cleanup/Settlement为准。

首切面只编译描述符和Graph，不实例化真实Plugin或Provider。

## 4. API、CLI与REPL

| 使用面 | 允许 | 禁止 |
|---|---|---|
| Go API | Build/Compile/Inspect/Explain/Diff/Conformance | 直调Kernel、签Permit、写Runtime/领域Fact |
| CLI | `validate`、`compile`、`inspect`、`explain`、`diff`的离线/只读包装 | 隐式联网、凭据探测、启动Run |
| REPL/交互壳 | 调用同一Go API并展示Diagnostic | 维护第二套语义或绕过Schema |
| TypeScript glue | 将稳定Schema映射给外部集成层 | 复制Go领域规则、持有Runtime权威 |

CLI/API输出必须可机读、确定性排序，并标注`candidate|observation|report`权威等级。真实外部Probe默认禁止；将来启用时必须显式命令、独立Operation和pre-run Evidence裁决。

## 5. Model Invoker桥

- 请求只携带`RouteID`与公开`routegateway`/`execution`/`union`合同允许的对象；
- 不依赖`model-invoker/internal`、厂商SDK、Raw/Native stream事件；
- stream、completed、cache usage和Provider状态只进入Observation；
- Tool Call只映射为ToolCall/FunctionCall Candidate Observation；Harness从精确Observation创建PendingAction，Tool Engine再从精确PendingAction创建领域ActionCandidate，任何前序对象都不能直接成为Tool Result或dispatch资格；
- 需要`ContextReference`物化时，Route明确支持且Binding current才允许；必需能力不支持则Fail Closed，可选能力只能形成有Owner和恢复条件的Residual。

## 6. 语言与性能

默认实现语言为Go；TypeScript仅作未来集成glue。当前没有经benchmark证明的计算稠密热点，不规划Rust。

若未来Go profile证明规范化、DAG编译或Schema校验无法满足经用户确认的目标，Rust提案必须同时给出：可复现benchmark假设、Go稳定API边界、FFI或进程隔离选择、panic/crash/timeout/Unknown语义、跨语言版本与回滚方案。未经该证据不得以“高性能”为由引入Rust。
