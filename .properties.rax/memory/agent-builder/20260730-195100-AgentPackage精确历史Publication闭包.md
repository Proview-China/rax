# AgentPackage 精确历史 Publication 闭包

- AgentPackage V1 预发布合同增加 create-once `AssemblyPublicationRefV2`；
- Compiler 不再自行拼装 artifact 坐标，统一调用 Harness `NewAssemblyPublicationBundleV2` 锁定 Publication 与四份 artifact refs；
- Loader 删除四个松散 artifact Reader，改用结构兼容 Harness `HistoricalReaderV2` 的单一窄接口；
- Loader 只接受 Harness Owner 已提交的 exact historical Bundle，并验证 Publication、input/compiler/frozen-time/handoff closure；
- 黑盒使用 Harness SQLite Publisher 与 AgentPackage SQLite Repository，覆盖双 Store restart、splice、missing 与 staged-but-uncommitted fail closed；
- 本切片不进入 Host、Runtime、Console，也不新增页面 API。
