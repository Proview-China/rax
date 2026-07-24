# Memory/Knowledge本地Current Reader完成

> 最终状态：**completed；第三次独立只读复审YES，P0/P1/P2=0**。targeted ordinary100/race20、full ordinary/race、vet、gofmt、diff、import与零网络验证全部通过；Owner-local边界不变，production root接线继续NO-GO。

## 审计修复增量

- 第二轮修复：`InspectAttempt`先进入Owner RLock再获取fresh clock，锁等待跨Attempt TTL返回`ErrNotCurrent`；AttemptCoordinate的RunID/TurnID逐项exact比较，AttemptInspection携带并回传RunID/TurnID，跨Run/Turn拒绝；
- `InspectForTurn`在Owner RLock内读取fresh clock；caller CheckedAt只保留因果输入，owner now过期或clock rollback均Fail Closed；
- `ReadContentExact`在同一RLock内执行S1 fresh clock/current/binding→Get→S2 fresh clock/current/binding/closure，锁等待/Get跨TTL或Get期间binding revision/RemoteLocator漂移均返回空body；
- RunID/TurnID进入Attempt、Request、Projection、Closure与Content Observation，S1/S2 exact一致，跨Turn replay拒绝；
- LocalAttempt/CurrentState/Projection/Inspection/Content Observation加入ContractVersion与ObjectKind并进入canonical digest；
- nested Source/Evidence/Projection refs逐项校验，duplicate semantic Record拒绝，Rank由Score/Record exact ref语义排序派生；
- Owner-local StatePlaneBinding canonical proof及TTL进入Projection/Closure/Observation；Reader接口封闭，两个Owner各自提供零网络内存StatePlane content store；
- `InspectAttempt`仅ID确实不存在返回`confirmed_not_persisted`，同IDrevision/digest漂移返回Conflict。

时间：2026-07-16 10:28（Asia/Shanghai）

## 本次闭合

- Memory与Knowledge分别新增Owner-local Current Reader，公开只读面固定为`InspectAttempt`、`InspectForTurn`、`ReadContentExact`；两个Owner使用独立package、类型、store与测试；
- 两个reference store均以expected-revision CAS保存不可变Attempt/current state版本，current只接受最新head，historical revision不可冒充current；
- Attempt复读覆盖原Observation/Result exact refs、canonical DomainResult、DomainResultAssociation与SettlementApplication；错误Association保持persisted但unsettled，不能形成turn projection；
- Memory current复读覆盖View/Watermark/Record/Projection、Tombstone与poisoning；Knowledge覆盖View/Snapshot/Pointer/Package/Record/Source/Projection、Withdraw、License/Trust/Conflict与poisoning；
- `ReadContentExact`仅使用注入的Owner-local reader并重算bytes length/digest，evicted或tampered bytes Fail Closed；返回projection、slice和body均深拷贝；
- Knowledge fixture暴露的canonical冲突已修复：seal负责规范化，clone只做形态保持的深拷贝，避免nil/empty被二次规范化后改变摘要；
- 64并发测试证明S1/S2期间只会得到一致旧快照或在current CAS后`ErrNotCurrent`，不会混合新旧Record/Snapshot/Pointer/body。

## 保留边界

- 零网络、零Retrieve、零Provider、零Resolver、零远程正文；未实现Retrieval Domain Gateway；
- 未新增Runtime、Context、Harness、Application Adapter，未连接Context production root；
- 首个G6B仍保持`MemorySources=0`、`KnowledgeSources=0`，两个Reader调用数为0；
- reference store只用于本地参考实现与测试，不代表生产Backend、持久State Plane或SLA。

## 实际验证

- 第二轮修复后`go test ./memory/contextsource ./knowledge/contextsource -count=100 -timeout=120s`：通过；
- 第二轮修复后`go test -race ./memory/contextsource ./knowledge/contextsource -count=20 -timeout=180s`：通过；
- 第二轮修复后`go test ./... -timeout=120s`与`go test -race ./... -timeout=180s`：通过；
- 第二轮修复后`go vet ./...`、`gofmt -d`、`git diff --check`、import/零网络扫描：通过；
- `go test ./memory/contextsource ./knowledge/contextsource -count=100`：通过；
- `go test -race ./memory/contextsource ./knowledge/contextsource -count=20`：通过；
- `go test ./...`：通过；
- `go test -race ./...`：通过；
- `go vet ./...`：通过。
- `gofmt -l .`：无输出；
- diff-check与尾随空白扫描：通过；
- import-boundary扫描：两个Reader只依赖自身`contract`与Go标准库，未发现Runtime/Foundation/Kernel、Context、Harness或其他Owner实现依赖；
- 禁止能力与生产接线扫描：未发现`Retrieve`方法、网络/Provider/Resolver依赖或Context production root引用。
