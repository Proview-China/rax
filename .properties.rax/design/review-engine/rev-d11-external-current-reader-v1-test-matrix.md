# REV-D11 External Current Reader V1 Conformance 与 Benchmark 设计

## 1. 状态

- 本文件冻结 **Review-owned reusable conformance/benchmark 设计**。
- Review-owned aggregate、S1/S2、closed recovery、TTL、clock rollback与deep-clone测试已经落地并通过最终独立复审；本文件中的N=1/8/64/256专用benchmark尚未全部实现。
- 跨 Owner production Adapter/certification与宿主root仍未冻结为可发布组合，不满足production实现门禁。
- fixture 只能模拟公开只读 Reader 行为，不得冒充 Binding/Evidence/Policy/Authority/Scope production Owner。

## 2. Reusable conformance 目标

未来套件只接受一个被测 `DecisionExternalCurrentReaderV1` 与 test-only 观察器，不接收任何 Owner write Port。

候选文件级落点：

- `ExecutionRuntime/review/conformance/external_current_reader_v1.go`：可复用 suite 与 fixture interface；
- `ExecutionRuntime/review/conformance/external_current_reader_v1_test.go`：reference adapter 自验；
- `ExecutionRuntime/review/tests/external_current_reader_v1_conformance_test.go`：黑盒入口。

Review-owned实现已按实际包结构落在`decisioncurrent`、`tests`与相关conformance文件；不得据此创建或宣称跨Owner production Adapter/root。

### 2.1 Fixture 最小能力

fixture 只允许提供：

1. Reader factory；
2. immutable canonical request/response seeds；
3. deterministic monotonic/rollback clock；
4. exact read call log；
5. read-only fault script（lost reply、Unavailable、Indeterminate、drift、TTL crossing）；
6. mutation-call count 必须恒为零。

Review聚合fixture不得持Binding publisher或Runtime Fact write capability；未来Runtime Binding Owner conformance必须单独覆盖publisher，但该写口绝不注入Review fixture/root。

## 3. Conformance 矩阵

| ID | 场景 | 预期 |
|---|---|---|
| C-01 | full Binding ref 六字段 exact replay | S1/S2 均用同一 canonical ref，成功 projection full-equal |
| C-02 | Binding 任一字段漂移 | `Conflict/BindingDrift` 或 `BindingSetConflict`，零 projection |
| C-03 | Evidence full ref 与 full OwnerFact 一一对应 | canonical 排序，输出无缺项/额外项 |
| C-04 | 相同 Evidence exact duplicate | 单项只读计划；aggregate digest/TTL不受重复影响 |
| C-05 | 同 Evidence Ref 换 classification/digest | `Conflict/EvidenceConflict`，Owner mutation=0 |
| C-06 | Evidence OwnerFact 任一字段漂移 | `Conflict/EvidenceConflict` 或 `OwnerConflict`，整批 zero |
| C-07 | Policy/ActorAuthority/ReviewerAuthority/Scope exact ref漂移 | 对应 closed conflict/precondition error，整批 zero |
| C-08 | S1后任一Owner revision/digest/current漂移 | S2检测，不能追随新revision，整批 zero |
| C-09 | S1/S2的ProjectionID/Revision/Digest、ExactSubject、Checked、Expires任一变化 | full immutable projection逐字段比较失败，整批zero；Review不补签digest |
| C-10 | 每个Owner分别为唯一最短TTL | aggregate expiry精确等于该Owner值 |
| C-11 | 每项Evidence分别为唯一最短TTL | aggregate expiry精确等于该Evidence值 |
| C-12 | `now == expires` / S2跨TTL | Fail Closed；零 current projection |
| C-13 | baseline为零、S1/S2/恢复期间clock rollback | `PreconditionFailed/ClockRegression` |
| C-14 | lost reply后原ctx取消 | recovery仍只Inspect同一canonical request；无mutation |
| C-15 | recovery第二次成功但已跨TTL | 仍失败；不能返回旧current |
| C-16 | recovery持续Unavailable/Indeterminate | closed Unavailable/Indeterminate；无第三次读 |
| C-17 | authoritative NotFound | exact项缺失；不改用current index或默认值 |
| C-18 | unknown被错误降为NotFound | conformance失败 |
| C-19 | Owner返回额外/缺失/重复Evidence | `Internal/InvalidCanonicalForm`或closed conflict |
| C-20 | mutable input/output alias | 调用后修改原slice/seed不改变sealed结果 |
| C-21 | 64并发相同request | 每个结果内部S1/S2一致，无data race/共享可变alias |
| C-22 | 不同Evidence规模并发 | canonical结果确定，错误优先级确定 |
| C-23 | 缺任一必需Reader capability | `CapabilityUnavailable/ComponentMissing` |
| C-24 | Reader尝试Owner mutation | suite立即失败；只读面不可携带write Port |
| C-25 | projection被当Authority/Evidence | API shape与测试均不得产生Permit/Fact/Admission |
| C-26 | Owner projection缺Checked或ProjectionDigest | `InvalidArgument/InvalidReference`或`InvalidDigest`，不得进入S1 |
| C-27 | `Checked<=0`、`now<Checked`、`Checked>=Expires`、`now>=Expires` | closed precondition error，零projection |
| C-28 | `ValidateCurrent`接受错误ProjectionRef/Ref/Subject或坏digest，或Owner current Reader未验证index却返回成功 | conformance失败；纯值验证与Reader current证明缺一不可 |
| C-29 | S1保存字段子集而非完整projection | conformance失败；S2必须比较full owner-sealed projection |
| C-30 | 同exact projection连续Inspect，调用now不同但都在TTL内 | 返回逐字段相同的deep clone；Checked/Expires/Digest固定 |
| C-31 | Reader用fresh now重封Checked/Expires/Digest | conformance失败；fresh now只能传给ValidateCurrent |
| C-32 | Owner状态变化 | create-once新projection并CAS current index；旧projection exact history不变，S2对旧expected ref Fail Closed |
| C-33 | projection自然到期 | historical exact Inspect仍返回原deep clone；ValidateCurrent失败且不创建刷新projection |
| C-34 | 状态变化覆盖旧projection或复用ProjectionID/Revision | conformance失败；append-only history与create-once被破坏 |
| C-35 | S1以full ExactSource+ExactSubject查询Owner线性化current index | 返回完整ProjectionRef后按该Ref exact Inspect；这是唯一成功路径 |
| C-36 | S1使用by-name/latest/list/filter或caller预带ProjectionRef | conformance失败；不得进入Owner projection读取 |
| C-37 | current index在S1 resolve与exact Inspect之间漂移 | Owner current Reader原子检查失败；纯值ValidateCurrent不得代替index证明，不追随新Ref，整批zero |
| C-38 | current index在S1完成后、S2前漂移 | S2按旧exact Ref复读并检测index不等，整批zero |
| C-39 | S2重新resolve并拼入新current Ref | conformance失败；S2只能使用S1保存的full Ref |
| C-40 | current index返回缺ID/Revision/Digest的partial Ref | closed InvalidReference/InvalidDigest；零projection |
| C-41 | current index从A切到B后再次发布旧A（ABA） | conformance失败；current index只能CAS前进到新create-once Ref |
| C-42 | resolve reply lost | 至多一次以同一ExactSource+ExactSubject exact resolve；不得改用latest/by-name |

## 4. Benchmark 设计

目标是建立 Go 实现的可重复成本基线，不设置生产 SLA，也不据此预选并发拓扑或 Rust。

候选文件：`ExecutionRuntime/review/conformance/external_current_reader_v1_benchmark_test.go`。实现门禁关闭前不创建。

### 4.1 Case

| Benchmark | 规模/变量 | 记录指标 |
|---|---|---|
| `BenchmarkExternalCurrentSeal` | Evidence N=1/8/64/256；Owner ProjectionDigest只验证不补签 | ns/op、B/op、allocs/op |
| `BenchmarkExternalCurrentS1S2` | N=1/8/64；每Owner一次S1 exact index resolve+Inspect，S2只exact Ref Inspect | ns/op、resolve/inspect reads/op、B/op、allocs/op |
| `BenchmarkExternalCurrentConcurrent` | parallelism=1/8/64，N=8/64 | ns/op、B/op、allocs/op；race另跑 |
| `BenchmarkExternalCurrentLostReply` | 首读Unavailable/Indeterminate，exact recovery成功 | reads/op、ns/op；必须恰好一次recovery |
| `BenchmarkExternalCurrentDriftFailClosed` | Binding/Evidence/Authority在S2漂移 | fail path ns/op、allocs/op；不得产生partial result |
| `BenchmarkExternalCurrentTTLMin` | 每项轮流为唯一最短TTL | ns/op、allocs/op；结果expiry精确 |

### 4.2 口径

- benchmark只测Review聚合、canonical sort/dedupe、Owner public `ValidateCurrent`调用、immutable projection deep clone/S1-S2 full compare、aggregate digest/min TTL与错误路径；不把模拟网络时延称为生产性能，也不测Review补签或按调用时间重封Owner digest。
- 每个 case 使用固定 seed 与 immutable fixture；`b.ReportAllocs()`；记录 exact Owner reads/op。
- ordinary benchmark 与 `-race` correctness 分开，race 数字不作为性能基线。
- 至少三次稳定运行后才允许用 `benchstat` 比较；回归阈值必须另经用户批准，当前不设 pass/fail SLA。
- 没有基准证明的计算稠密热点，不引入 Rust、FFI 或独立进程。

## 5. 未来验证命令口径

仅在实现获得授权后使用：

```text
go test ./... -run 'ExternalCurrent.*Conformance' -count=100
go test -race ./... -run 'ExternalCurrent.*Conformance' -count=20
go test ./... -run '^$' -bench 'ExternalCurrent' -benchmem -count=3
go test ./...
go test -race ./...
go vet ./...
```

当前波次只运行现有 Review module 门禁，不把未创建的 suite/benchmark 声称为已通过。

## 6. Binding Authoritative Current 第三候选门禁

完整第四候选见 [`rev-d11-binding-authoritative-current-port-delta-v1.md`](rev-d11-binding-authoritative-current-port-delta-v1.md)。以下28例是Binding P0关闭前的hard negatives；每例都必须进入targeted ordinary100、targeted race20、full ordinary与full race。`go vet`只作静态补充。

| ID | 场景 | Oracle | ordinary100 | race20 | full/race | vet |
|---|---|---|:---:|:---:|:---:|:---:|
| BIND-01 | Set+全部Facts renew，projection/index未推进 | authoritative closure复读后BindingDrift、zero | ✓ | ✓ | ✓ | ✓ |
| BIND-02 | selected Fact revoke，index未变 | zero current | ✓ | ✓ | ✓ | ✓ |
| BIND-03 | 非selected member Fact revoke/expire | all-member closure失败 | ✓ | ✓ | ✓ | ✓ |
| BIND-04 | Set侧Grant更新、Fact侧旧值 | full Grant mismatch、zero | ✓ | ✓ | ✓ | ✓ |
| BIND-05 | Fact侧Grant更新、Set侧旧值 | full Grant mismatch、zero | ✓ | ✓ | ✓ | ✓ |
| BIND-06 | selected capability缺失或重复 | PreconditionFailed/BindingNotCertified | ✓ | ✓ | ✓ | ✓ |
| BIND-07 | 非selected Fact为唯一最短TTL | expiry精确取该值 | ✓ | ✓ | ✓ | ✓ |
| BIND-08 | Set侧非selected Grant为唯一最短TTL | expiry精确取该值 | ✓ | ✓ | ✓ | ✓ |
| BIND-09 | Fact侧非selected Grant为唯一最短TTL | expiry精确取该值 | ✓ | ✓ | ✓ | ✓ |
| BIND-10 | index回旧Ref但highestRevision更高 | Conflict/RevisionConflict | ✓ | ✓ | ✓ | ✓ |
| BIND-11 | historical `active,true`被当current | historical可读、current拒绝 | ✓ | ✓ | ✓ | ✓ |
| BIND-12 | Resolve lost reply后底层renew | recovery只能开启新S1 | ✓ | ✓ | ✓ | ✓ |
| BIND-13 | snapshot读取期间clock rollback | zero、ClockRegression | ✓ | ✓ | ✓ | ✓ |
| BIND-14 | Identity literal golden | canonical JSON与ProjectionID逐字命中冻结golden | ✓ | ✓ | ✓ | ✓ |
| BIND-15 | Closure literal golden | canonical JSON与ClosureDigest逐字命中冻结golden | ✓ | ✓ | ✓ | ✓ |
| BIND-16 | `binding_set`被改名为`bindingSet`/`binding_set_ref` | `InvalidArgument/InvalidCanonicalForm`；zero | ✓ | ✓ | ✓ | ✓ |
| BIND-17 | Identity或Closure任一exact字段缺失/重复 | 缺字段为`InvalidArgument/InvalidCanonicalForm`或Validate失败；重复key为`Conflict/DuplicateCanonicalKey`；zero且不产生ID/digest | ✓ | ✓ | ✓ | ✓ |
| BIND-18 | 首建成功 | receipt/history/highest/current四对象Revision=1原子全有 | ✓ | ✓ | ✓ | ✓ |
| BIND-19 | 首建任一stage失败 | 四对象及底层mutation全无 | ✓ | ✓ | ✓ | ✓ |
| BIND-20 | 64并发同ExpectedCurrent续版 | 一个成功；loser零sidecar/底层mutation | ✓ | ✓ | ✓ | ✓ |
| BIND-21 | Create/CAS lost reply | 仅Inspect exact PublishRef；NotFound/Unknown不重调mutation | ✓ | ✓ | ✓ | ✓ |
| BIND-22 | same PublishRef换Input/digest | Conflict；历史闭包不变 | ✓ | ✓ | ✓ | ✓ |
| BIND-23 | history有效但current index缺失/坏/指向他项 | historical exact仍可读；current失败 | ✓ | ✓ | ✓ | ✓ |
| BIND-24 | 纯时间跨Expires | ValidateCurrent失败；Revision/publish count不变 | ✓ | ✓ | ✓ | ✓ |
| BIND-25 | bound association revoke/漂移 | S1/S2 zero；caller不能替换association | ✓ | ✓ | ✓ | ✓ |
| BIND-26 | consumer Binding/capability revoke或换Ref | S1/S2复读失败；zero | ✓ | ✓ | ✓ | ✓ |
| BIND-27 | association唯一最短TTL | Projection expiry精确取该值 | ✓ | ✓ | ✓ | ✓ |
| BIND-28 | consumer Binding唯一最短TTL | Projection expiry精确取该值 | ✓ | ✓ | ✓ | ✓ |

补充职责oracle：BIND-01至BIND-28中，projection/association index、highestRevision与实时Binding/consumer closure只能由Owner Readers证明；纯值ValidateCurrent只验sealed projection、expected与fresh TTL。仅调用纯值方法的fixture必须Fail Closed。

额外结构oracle：ctx取消零部分结果、deep-clone无alias、64并发无data race、Review Reader mutation call count恒为零、Review静态类型无Publisher、缺Owner snapshot/association capability构造期Fail Closed。

未来命令口径：

```text
go test -count=100 ./... -run 'ReviewBindingAuthoritativeCurrent'
go test -race -count=20 ./... -run 'ReviewBindingAuthoritativeCurrent'
go test ./...
go test -race ./...
go vet ./...
```

当前仅有资产候选，没有Runtime/Review Go实现，上述命令未运行且不得宣称通过。
