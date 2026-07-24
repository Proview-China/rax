# CTX-D06 Continuity公共Reader缺口收窄

时间：2026-07-17 15:00:00 +08:00

Context Owner只读核对live Continuity后确认：`TimelineOwnerFactRefV1`已经携带Owner Fact与Payload的exact revision/digest/scope，Checkpoint V2已经携带Context Generation/Frame exact refs，旧资产所称“generic Timeline骨架不足以表达结构共享恢复”不再准确。

当前唯一Delta是Continuity typed Owner-current Reader/Router仍定义在`continuity/runtimeadapter`而非公共`continuity/ports`。Context不能导入该实现包，也不能复制第二套nominal或用composition快照冒充current。后续须由Continuity Owner发布同语义公共Port或唯一无损facade；Context只在获批后实现回读自身Frame/Generation/Outcome exact-current的只读Adapter。

边界保持不变：Context不创建Evidence/Event，不分配Timeline sequence，不写SQLite/RocksDB；Continuity拥有Evidence复读、事件顺序、publish、storage、Checkpoint/Fork/Rewind与lost-reply恢复。公共Reader与production composition root未闭前，真实Timeline投影NO-GO。
