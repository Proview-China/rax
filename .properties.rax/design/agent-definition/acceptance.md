# AgentDefinition V1 验收

## 1. 白盒与单元测试

- YAML 安全子集逐项拒绝：重复键、merge、anchor、alias、tag、非字符串键、float、timestamp、多文档、未知字段。
- strict JSON round-trip 与 canonical digest 稳定；map 顺序、nil/empty 策略和集合排序有 property/fuzz 测试。
- Definition create-once、revision、current、撤销和过期状态机。
- 明显 secret/path、绝对路径、未注册 required extension 零写入；正常相对标识字符串不误拒。
- 同请求回包丢失后 exact Inspect；同 ID 换内容 Conflict。
- 自定义 namespaced kind/capability/schema 可通过治理目录接入，无硬编码 switch。

## 2. 黑盒测试

给定一份最小合法 YAML：

1. 解析为 Source；
2. Owner Seal 为 Definition；
3. 输出严格 JSON 和 exact ref；
4. 重复执行得到相同内容摘要；
5. 全过程不访问网络、不读取 secret 值、不启动组件。

给定任一首版 6+1 能力缺失、非 production 或 residual 无 Owner 的配置，必须在装配前失败且副作用计数为 0。

## 3. 故障与并发

- 64 并发相同 create：只线性化一个对象，其余取得 exact 同结果。
- 64 并发同 ID/revision 不同内容：只允许一个内容胜出，其他 Conflict。
- create 回包丢失：Inspect 找回原对象，不产生 revision 2。
- clock rollback、过期审批、过期有效窗口：fail closed。
- Approval S1/S2 同 exact ref 全字段一致；慢 S2 Reader 跨 TTL、S1/S2 drift、S2 后 clock rollback 全部零写。
- conformance 覆盖 changed-content conflict、revision CAS、lost reply exact recovery、revoke/expire、clock ABA、clone 与 typed-nil；通过该窄 testkit 不等于获得 production durability/SLA 认证。

## 4. 首版退出标准

- 解析器不能被 YAML 特性绕过 strict JSON 语义；
- Definition 不含任何 Owner 实现 DTO 或 Provider 句柄；可信 Secret 仅为 ref，unknown optional opaque 不得被视为无秘密或可信生产输入；
- Definition 可以表达全部 6+1 与自定义组件；
- Runtime、Harness、组件均不需要 import `agent-definition` 实现包；
- 只有 Agent Assembler 公共入口消费 sealed Definition。
