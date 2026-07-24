# AgentAssembler SQLite WAL计划仓完成

- `repository.SQLiteV1`实现既有`ResolvedAgentPlanRepositoryV1`，未改变ResolvedPlan/current digest或Memory语义。
- 同库保存immutable Plan history与current projection history/pointer；strict CAS、schema digest、复合外键、row digest和严格JSON均Fail Closed。
- 验证覆盖Ensure/CAS lost reply后重启Inspect、64独立Store、stale CAS、历史Plan ABA、row/schema损坏、typed nil，以及target100/race20/full ordinary/race/vet。
- Catalog/Facts只有公开exact Reader、没有Owner Repository写口，因此未持久化也未另造Owner；production仍需外部Reader。能力边界为单节点本机crash durability，无HA/SLA。
