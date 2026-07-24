# Context Engineering SDK联合候选

时间：2026-07-17 23:25（Asia/Shanghai）

用户确认下一阶段把Prompt开发入口与Outcome评测入口一起设计，并采用可插拔Evaluator/Policy：Context不定义唯一评分公式；本地规则、回放、人工或未来受治理远程Judge均可提供Observation，Context只在exact复核后形成Evaluation/Feedback Fact。

本轮新增`design/context-engine/engineering-sdk.md`并同步主合同、plan、test matrix和coverage matrix。候选保持既有Offline SDK六operation闭集不变，首切面规划独立typed `ContextEngineeringSDKV1`：Prompt validate/preview、Evaluation prepare/admit、Feedback build。远程Judge、production publish、API server/root、Capability与跨Owner接线仍不实现。

当前状态为design/plan review pending；待用户审核exact DTO、limits和typed-only首切面后才写Go。
