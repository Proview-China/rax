# AgentPackage Repository + Exact Loader V1

## 1. 冻结边界

本切片只让已编译的 `AgentPackageV1` 可被持久保存并按 exact ref 重读，同时由 Loader 按锁定的 `AssemblyPublicationRefV2` 重新读取 Harness Owner 已提交的完整历史 Bundle。锁清单只是待验证声明，不是 Publication 已提交或可运行的证据。

## 2. Repository

- SQLite durable store 使用 WAL、`synchronous=FULL`、外键与 bounded busy timeout；
- 坐标为 `package_id + revision`，内容为完整 Package JSON 与 row digest；
- create-once：首次合法写入成功；完全相同重复写幂等；同坐标不同 digest 或 body 返回 Conflict；
- Inspect 必须提供 `package_id + revision + digest + contract/schema version`，返回前重算 row digest、Package digest 并验证 exact ref；
- 写回复丢失时只 Inspect 原 Package ref；没有原写入证据则返回 NotFound/Indeterminate，禁止换 ref、换 attempt 或伪造成功。

## 3. Exact Loader

Loader 只依赖两个窄 Reader：Package exact reader 与结构兼容 Harness `HistoricalReaderV2` 的 Publication reader。它按锁中的 exact Publication ref 一次重读权威 Bundle，调用 Harness `Validate` 重算 Publication 与 artifact canonical digest，并检查：

- Publication ref 及四个 artifact refs 与 Package lock 完全一致；
- Generation/Manifest/Graph input 与 Package lock 一致；
- Generation compiler/frozen time 与 Package lock 一致；
- Handoff generation/manifest/graph 与同一闭包一致；
- 任一 NotFound、未提交、body drift、lock drift 或 ID/revision/digest splice 均 fail closed。

Loader 只接受 sealed Generation。它返回深拷贝的 verified closure，但该闭包仍不是 executable/authorized runtime object；不实例化 Factory、不启动 Host/Runtime，也不授予 activation、dispatch 或 production/SystemReady。

## 4. 验收

- SQLite 关闭重开后 exact Inspect 保持一致；
- 同内容并发 create 只有一个 durable truth，所有调用得到同一 Package；
- 同坐标不同内容并发只能 Conflict，不能覆盖；
- 写回复丢失仅凭原 ref 的 exact Inspect 恢复；缺证据不成功；
- 真实 Harness SQLite Owner 与 Package SQLite Store 关闭重开后仍恢复同一 exact closure；
- Publication/artifact splice、body/lock drift、仅有 lock、仅 staged 未 commit 均被拒绝；
- ordinary、race、vet、blackbox、diff-check 全绿。

## 5. 明确不做

不导入或实现 Harness Publication Store，不实例化 Factory，不启动 Host/Runtime，不新增 Console/TS DTO，不声明 production readiness。
