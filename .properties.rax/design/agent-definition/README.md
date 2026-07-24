# Agent Definition V1

## 1. 当前裁决

- 状态：设计已确认；P1-P4 owner-local/reference 实现候选已落地并完成本轮返修，等待独立终审。生产持久 Backend 与 trusted extension validator 尚未落地。
- 首版目标：用一份声明式配置表达一个完整 Agent，并把 6+1、Harness、Application、Runtime 的需求一次钉死。
- 作者格式：YAML；进入系统后的唯一语义格式：严格 JSON 对象树。
- 本模块只拥有“Agent 想要什么”，不解析组件、不创建实例、不启动任何执行。

当前代码尚不能通过简单配置直接构建完整 Agent：现有 Harness 接受的是程序化 `AssemblyInputV1`，Runtime 旧 `ResolvedAgentPlan` 又不足以表达完整装配。V1 设计补的是最上游声明合同，不把已有局部能力伪装成生产可用整体。

## 2. 对象分层

```text
用户 YAML
  -> AgentDefinitionSourceV1       仅表示通过严格解析的作者输入
  -> AgentDefinitionV1             Definition Owner 封存的权威声明
  -> AgentDefinitionRefV1          下游只携 exact ref
  -> Agent Assembler               唯一解析者
```

作者不能提供或覆盖 `Digest`、Owner 时间、审批事实和解析证据；这些字段由对应 Owner 生成。可信 Secret 字段只能使用引用。opaque extension 只做明显 secret/path 粗筛，unknown optional 始终视为 untrusted，不能以黑名单通过证明“不含秘密”。

## 3. 6+1 首版能力集合

以下能力均为首版必需项，最终系统验收时必须处于 `production`，不能以 fake、owner-local、standalone 或 reference 实现代替：

| 能力域 | Definition 表达 | 最终生产硬门 |
|---|---|---|
| continuity | checkpoint、restore、timeline 策略引用 | 可持久化事实、独立 Inspect、恢复不伪造外部回滚 |
| tool + MCP | tool surface、MCP、credential、effect 策略引用 | 双门治理、actual-point enforcement、unknown 只 Inspect |
| memory + knowledge | retrieve、candidate、commit、forget 策略引用 | 正式提交由领域 Owner CAS，外发先 Effect |
| sandbox | isolation、placement、resource、network 策略引用 | Runtime Activation 前存在生产 Environment Provider |
| review | automatic、human、TTL、policy 引用 | Verdict exact 绑定 candidate/intent/scope/authority |
| context + cache | source、partition、prompt、refresh 策略引用 | Context Owner 产 exact Frame，cache 命中重新鉴权 |
| Harness | execution stack、model profile、route 引用 | 只拥有 Run 内 Session/Event，不拥有 Runtime Outcome |

## 4. Owner 边界

Agent Definition 拥有：

- 字段、版本、canonical、摘要和 immutable revision 语义；
- AgentIdentity、Profile、策略、组件需求和秘密引用的表达；
- 来源、审批引用、有效窗口和变更原因；
- 严格解析与结构化诊断。

Agent Definition 不拥有：

- Component Manifest、Component Release 或 Binding 事实；
- Prompt、Memory、Review、Sandbox、Tool 等领域事实正文；
- Secret 值、Provider 句柄、网络连接、文件系统路径注入；
- Instance、Run、Lease、Permit、Settlement 或 ExecutionOutcome；
- 任何组件的生产构造器和生命周期。

## 5. YAML 到严格语义树

V1 YAML 仅是作者体验层。解析器必须在零外部副作用阶段完成：

- 拒绝重复键、未知字段、非字符串键和多文档尾随内容；
- 拒绝 merge key `<<`、anchor、alias、自定义 tag；
- 拒绝浮点、NaN、Infinity、隐式 timestamp 和实现相关 scalar；
- 只允许 null、boolean、受范围限制的整数、字符串、数组、对象；
- 数组是否是集合逐字段冻结；集合稳定排序、去重，顺序数组保持原序；
- `nil` 与空数组是否等价逐字段定义，不使用语言默认值猜测；
- 转成严格 JSON 语义树后再按 Runtime canonical 规则摘要。

详细字段见 [contract-v1.md](contract-v1.md)，验收见 [acceptance.md](acceptance.md)，架构图见 [architecture.drawio](architecture.drawio)。

## 6. 与下游的关系

- Agent Assembler 只接受 sealed `AgentDefinitionV1`，不能直接接受 YAML。
- Runtime 不读取、解释或迁移 Definition，只消费已冻结的装配产物。
- Harness 不读取 Definition，只接收由 Assembler 生成的 `AssemblyInputV1`。
- agent-host 可调用 Definition Decoder，但不能修改字段或补签事实。
- 自定义组件使用 namespaced kind/capability/schema/extension；Definition 不增加硬编码 switch。

## 7. 进入 Plan 的门

- [x] 用户确认字段、YAML 子集和版本策略；
- [x] 用户确认首版 6+1 全部为生产硬门；
- [x] Agent Assembler 的 `ResolvedAgentPlanV1` 与 Component Release Catalog 同时确认；
- [x] agent-host 唯一 Composition Root 同时确认；
- [x] 黑白盒、故障、并发与自定义组件验收边界确认。
