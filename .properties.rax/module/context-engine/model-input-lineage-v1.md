# Context Model Input Lineage Current V1 模块说明

本能力把一个Context-owned模型输入物料的exact Ref，与其内部密封的独立
Context Frame exact Ref组成可复读的current lineage projection。

调用方提交Material exact source和观察上限；Reader使用Material exact/current
Reader与Frame exact-current Reader完成两轮读取，只在两轮结果完全一致且仍处于
最小TTL窗口时返回projection。

该模块不修改模型输入内容，不调用Provider，也不授予dispatch权限。目前代码不含
production Frame backend或composition root；测试fixture只用于验证合同和并发行为，
不得解释为生产State Plane或SLA。
