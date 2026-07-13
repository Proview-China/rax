# Intent、Mechanism、Effect与Profile路由设计形成

- 时间：2026-07-12 23:19:04 CST
- 模块：`model-invoker`
- 阶段：执行语义并集详细设计草案，等待用户共同审核

## 本次进展

1. 将并集语义扩展为`IntentGraph → MechanismPlan/Attempt → EffectRecord/Verification`因果链；
2. 明确统一上层意图和真实Effect，不统一底层工具方言；
3. 定义`ModelBehaviorProfile × HarnessCapabilityProfile × RuntimePolicy`合成EffectiveProfile；
4. 定义硬过滤、偏好排序、Effect可观测性、Verifier和安全fallback；
5. 定义文件操作、Structured Output、Tool Call、代码执行和Computer Use首批原语；
6. 文件Effect由真实文件系统前后快照、hash和diff产生；
7. Structured Output区分native strict、JSON Object和emulated strict链；
8. Tool Call、Tool Execution与Tool Effect分离；
9. Event新增intent、mechanism和effect family；
10. Result把执行status与verification status分离；
11. 当前没有修改Runtime公共类型、创建模块或实现代码。

## 当前资产

- [`intent-mechanism-effect-profile-routing-v1-draft.md`](../../design/model-invoker/intent-mechanism-effect-profile-routing-v1-draft.md)
- [`execution-semantic-union-v1-draft.md`](../../design/model-invoker/execution-semantic-union-v1-draft.md)

## 下一步

与用户审核15项核心决定和五类首批原语；确认后再做GPT/Codex、Claude SDK与caller-hosted Route纸面编译示例，设计未确认前不进入实现计划。
