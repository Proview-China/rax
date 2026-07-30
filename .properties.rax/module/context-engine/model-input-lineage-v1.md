# Context Model Input Lineage Current V1 模块说明

本能力把一个Context-owned模型输入物料的exact Ref，与其内部密封的独立
Context Frame exact Ref组成可复读的current lineage projection。

调用方提交Material exact source和观察上限；Reader使用Material exact/current
Reader与Frame exact-current Reader完成两轮读取，只在两轮结果完全一致且仍处于
最小TTL窗口时返回projection。

该模块不修改模型输入内容，不调用Provider，也不授予dispatch权限。Context现已提供
`framestore.SQLiteV1`单节点durable Owner backend：Material lineage可注入该Reader，
并由构造器核对Frame Reader的Context Owner binding。Store以append-only history、
Generation current expected-CAS、完整operation ledger、S1/S2和fresh clock/TTL证明
Frame exact ref仍current。

该能力仍没有Application/Harness/Model production composition root、Capability、
Continuation或Turn推进，也不声明多节点HA、备份与SLA。线程安全fixture只用于测试，
不得替代durable store或解释为生产State Plane。
