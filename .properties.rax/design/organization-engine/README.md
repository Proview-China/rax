# Organization Engine 设计骨架

## 设计状态

- 当前阶段：设计草案。
- 当前授权：仅允许共同讨论和完善设计，不存在实现授权。
- 本文件不是冻结合同，也不代表已经创建 Organization Engine 模块。

## 作用

Organization Engine 表达 Agent 集群中的主体、位置、职责、职权和问责关系。
它把组织设计编译为可引用的组织快照和权限约束，但不替代 Runtime 强制执行。
组织语义固定拆分为 Identity、Role、Responsibility、Authority、Accountability 五层。

## 五层模型

- `Identity`：稳定标识一个人、Agent、服务或受控主体，回答“是谁”。
- `Role`：描述主体在组织拓扑中的位置和功能模板，回答“处于什么位置”。
- `Responsibility`：描述必须完成、维护或守护的结果，回答“应负责什么”。
- `Authority`：描述允许作用的资源、动作、范围和期限，回答“被允许做什么”。
- `Accountability`：绑定结果归属、证据、复核和追责主体，回答“由谁承担后果”。

五层不得压缩成一个 Prompt Role，也不得仅依赖自然语言约束。

## 核心输入

- `IdentityDefinition[]`：主体类型、稳定身份和生命周期引用。
- `RoleDefinition[]`：角色模板、组织位置和关系约束。
- `ResponsibilityDefinition[]`：目标、义务、范围和完成条件。
- `AuthorityPolicy[]`：资源、动作、期限、委托和禁止项。
- `AccountabilityBinding[]`：归属主体、证据要求、复核和升级路径。
- `OrganizationAssignment[]`：五层对象之间的版本化绑定。

## 核心输出

- `OrganizationSnapshot`：某一时点的不可变组织关系快照。
- `AuthorityGrantSet`：供 Runtime、工具网关和 Sandbox 验证的职权集合。
- `AccountabilityMap`：动作、结果、责任主体和证据要求的映射。
- `ContextProjection`：允许进入 Agent Context 的组织语义投影。

## 本引擎拥有

- 五层组织概念及其版本化绑定语义。
- 组织拓扑、上下级、协作、委托和升级关系的表达。
- Authority 与 Responsibility 分离，不以职责自动推导职权。

## 本引擎不拥有

- 不直接执行 Authority，不修改 Sandbox 或工具权限。
- 不通过 Prompt 代替 Runtime 的硬性权限验证。
- 不作 Review 判断，不提出 Management ControlIntent。
- 不调用模型、工具、MCP，也不写入业务记忆。

## 与 Runtime 的关系

AgentDefinition引用组织定义，装配阶段解析为固定`OrganizationSnapshot`。
Runtime 在实例化和每次敏感动作前验证有效 `AuthorityGrantSet`。
Role 和 Responsibility 可经 Context Engine 投影给 Agent；Authority 仍由系统强制执行。
Runtime 产生的 Effect 与证据回填 Accountability 链，但不能反向篡改历史快照。

## 依赖

- 稳定主体身份、租户边界和版本化引用合同。
- AgentDefinition、AgentIdentity、AgentInstance和AgentRun身份模型。
- Event、Effect、Evidence 与 Artifact 的统一关联语义。

## 待共同决定

- Identity 是长期 Agent 身份还是实例身份，二者如何关联。
- Responsibility 的完成条件和跨 Agent 共同责任表达。
- Authority 的继承、收窄、临时提升、撤销和过期语义。
- 组织快照在长时间AgentRun或AgentSession中发生变化时如何处理。

## 进入 Plan 阶段的门槛

- 冻结五层模型及其互相禁止隐式推导的不变量。
- 冻结 OrganizationSnapshot 与 AuthorityGrantSet 的 v1 候选合同。
- 完成组织定义到 Context、Tool、MCP、Sandbox 权限的映射表。
- 准备越权、角色冲突、责任缺失和问责断链验收用例。
- 获得用户对范围、产物和实施顺序的明确审核与授权。
