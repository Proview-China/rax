# AgentDefinition SQLite WAL持久仓完成

- `store.SQLiteRepositoryV1`实现既有`DefinitionRepositoryV1`，未改变Definition digest、current projection或Memory语义。
- 同库保存immutable history、current exact绑定、current revision/state、highest checked与revoke closure；schema digest、复合外键、row digest、严格JSON均Fail Closed。
- 验证覆盖lost create reply后重启Inspect、64独立Store、clock rollback、expired ABA、revoke、row/schema损坏、typed nil，以及target100/race20/full ordinary/race/vet。
- 能力边界为单节点本机crash durability；无HA、远程副本、RPC或SLA。真实Approval current Reader仍由外部Owner提供。
