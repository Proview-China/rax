# 治理与控制设计域

## 1. 目标

把组织身份、职责、职权、审核判断和管理控制转化为Runtime可以验证和执行的显式合同，使“谁生产、谁负责、谁报告、谁修正”成为机器可追溯事实。

## 2. 当前模块

| 模块 | 核心职责 |
|---|---|
| [`organization-engine`](../../../design/organization-engine/README.md) | Identity、Role、Responsibility、Authority与Accountability |
| [`review-engine`](../../../design/review-engine/README.md) | 对过程和产物形成审核结论、证据与建议 |
| [`management-engine`](../../../design/management-engine/README.md) | 把目标和审核结论转换为带授权的管理意图 |
| Runtime Control Plane | 验证并执行暂停、恢复、批准、拒绝、纠偏和终止命令 |

## 3. 核心原则

- Prompt中的角色描述不是权限；真实Authority必须由硬门禁执行；
- Review负责判断，不直接改写Runtime内部状态；
- Management负责提出控制意图，不绕过权限和Runtime状态机；
- Runtime执行命令但不替Review作语义判断；
- 每项控制必须记录actor、reason、scope、precondition、command ID和结果；
- 纠偏输入作为带来源的新事件进入，不允许静默Prompt注入。

## 4. 待共同决定

- 审核是同步栅栏、异步观察者还是两种都支持；
- 人类、Agent和自动Policy的控制优先级；
- Authority委派、临时授权、撤销和过期规则；
- 多Agent组织中的上级、同级、代理和责任转移合同。

