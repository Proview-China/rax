# Knowledge Owner-local Current Reader

本包实现`KnowledgeContextSourceCurrentReaderV1`的本地只读面：

- `InspectAttempt`：进入Owner RLock后读取fresh clock，按Tenant、Execution Scope、Run/Turn、Attempt、Request、Idempotency、Observation与Result exact坐标复读本地Journal，并在Inspection回传Run/Turn；
- `InspectForTurn`：进入Owner一致性锁域后读取fresh owner clock，复读settled Attempt与Knowledge current View/Published Snapshot/Pointer/Package/Record/Source/Projection，形成绑定Run/Turn和State Plane proof的短TTL canonical closure；
- `ReadContentExact`：同一RLock内执行S1 fresh clock/current/binding、Get、S2 fresh clock/current/binding/closure复读，通过后才返回exact bytes；
- `PutAttempt`与`PublishCurrent`：内存reference store的expected-revision CAS，用于本地参考实现与测试，不是生产Backend或SLA。
- `StatePlaneContentStore`：包内封闭、零网络的Owner-local exact bytes参考实现；外部包不能注入自定义reader。

包内没有Retrieval、网络、Provider、Resolver、Context Adapter或production composition root。跨Turn/版本/type-pun、clock rollback、锁等待/Get跨TTL、binding漂移、historical Pointer、Withdraw、stale Projection、License/association漂移、evicted/tampered content均Fail Closed。
