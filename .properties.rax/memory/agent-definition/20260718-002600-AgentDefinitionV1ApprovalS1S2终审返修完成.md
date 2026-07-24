# AgentDefinition V1 Approval S1/S2 终审返修完成

- 时间：2026-07-18 00:26 +08:00
- 范围：Definition owner 代码、design、plan、module 与 memory；未 stage/commit。
- P0：新建写入采用同一 exact ApprovalRef 的 S1/S2 双读。S1/S2 全字段一致；S2 返回后取 fresh clock，拒绝回拨并重新验证 TTL；Definition `CreatedUnixNano` 使用 S2 clock。
- 反例：慢 S2 Reader 跨 TTL、S1/S2 drift、clock rollback 均零写；另验证 S1/S2 使用同一 exact ref 和 Owner 时间坐标。
- Extension 边界：明显 secret/path 继续 fail closed，但黑名单不宣称识别任意秘密。unknown optional 只 opaque/untrusted 保留，不得进入 trusted production resolution；key 注册与自洽 digest 不等于 exact schema trust，未来仍需治理目录绑定与 validator Port。
- Conformance：扩到 changed-content conflict、revision CAS、lost-reply exact recovery、revoke/expire、clock ABA、clone 与 typed-nil；明确不认证 production durability、availability、跨进程 CAS 或 SLA。
- 验证：ordinary100、race20、full ordinary/race、vet、import/link、contract fuzz、decoder fuzz 全部通过；Agent Assembler 单轮 link test 通过。
