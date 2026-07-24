# Result Bundle Current Grounding V2 测试矩阵

## 1. 状态

- 本矩阵对应的owner-local Go已落地，覆盖compound admission、full aggregate、S1/S2 drift、Binding drift、min TTL/TTL crossing、clock rollback、cancel/Unknown exact retry、deep clone与closed router；最终独立复审P0/P1/P2=0。production external Owner/root仍NO-GO。
- `RB-OWN-*`由对应Owner reusable conformance承担；`RB-REV-*`由Review consumer承担；`RB-ROOT-*`由宿主integration/system承担。
- Fake仅用于testkit；不得以Fake成功声明production backend/root。

## 2. canonical与Owner边界

| ID | 层级 | 场景 | Oracle |
|---|---|---|---|
| RB-REV-01 | unit | V2 Bundle含Request/Target、typed Artifacts、Anchors、Environment、Context、Validation Scope、full Evidence | Seal/Validate成功，literal JSON与golden digest稳定 |
| RB-REV-02 | unit | Artifacts/Anchors/Claims/Evidence输入乱序 | Seal canonical排序；重复项拒绝 |
| RB-REV-03 | unit | Claim Artifact只改Owner、Kind、Tenant、Revision、Digest或Scope之一 | Conflict，零Bundle写 |
| RB-REV-04 | unit | Claim Anchor不在对应Artifact Anchor set | Conflict，零Bundle写 |
| RB-REV-05 | unit | 声明Anchor未被任何Claim引用 | InvalidArgument，零Bundle写 |
| RB-REV-06 | unit | Claim Evidence使用弱string/digest或缺full字段 | InvalidArgument，零Bundle写 |
| RB-REV-07 | unit | Context Sources与Envelope Materials少一项/多一项/换digest | S1失败关闭，零Verdict |
| RB-REV-08 | unit | Environment ref type-pun Validation Scope ref | 编译shape/nominal测试拒绝；无通用转换helper |
| RB-REV-09 | whitebox | Continuity ArtifactRelation映射为generic Artifact current | Router拒绝undeclared/type-pun，零Verdict |
| RB-REV-10 | whitebox | Sandbox Snapshot ref当任意Artifact或Environment current | 只允许声明的typed adapter；其他kind拒绝 |
| RB-OWN-01 | conformance | locator wrapper valid但Artifact revision中不存在该locator | Artifact Owner closed `PreconditionFailed` |
| RB-OWN-02 | conformance | 同stable Artifact/同revision换digest | Conflict，history/current零覆盖 |
| RB-REV-10A | unit | Bundle自报OriginalTask/Acceptance digest或换掉Context exact source | 无此弱字段；OriginalIntent/AcceptanceCriteria必须exact匹配Envelope materials，zero Bundle/Verdict |
| RB-REV-10B | unit/golden | aggregate省略或修改Validation Scope Owner Association full projection | `Validate/Seal`失败；association Ref/Subject/Owner/Checked/Expires/Digest全部参与aggregate golden digest |
| RB-REV-10C | unit/golden | aggregate省略某个route Proof，或只保留Route ID/Reader interface | `Validate/Seal`失败；每个Artifact、Environment、Scope都必须保存nominal Declaration/full Owner+RouteRef+ReaderBindingRef/adapter digest proof |
| RB-OWN-02A | contract/golden | 三个具名ProjectionIdentityInput使用冻结literal JSON | canonical bytes与派生ID逐字稳定；改字段名/tag、缺字段、anonymous map/Subject整体输入全部拒绝 |
| RB-OWN-02B | conformance | Artifact/Environment只改Anchor/lease/current状态，exact source不变 | stable ProjectionID不变，只允许revision推进；SubjectDigest/ProjectionDigest变化 |
| RB-OWN-02C | conformance | Validation Scope只改Owner/coverage/evidence/revision，Owner-neutral Source不变 | stable ProjectionID不变；Owner换绑必须先推进association revision，旧history不覆盖 |
| RB-OWN-02D | conformance | 两个Owner分别声明相同Validation Scope `Kind/TenantID/ID` | association create/CAS只有一个winner；第二Owner `Conflict + BindingDrift`，root零双注册 |

## 3. current、S1/S2与ABA

| ID | 层级 | 场景 | Oracle |
|---|---|---|---|
| RB-OWN-03 | conformance | 首个projection create-once并CAS history/highest/current | 原子全有全无，revision=1 |
| RB-OWN-04 | conformance | 状态变化并发发布同next revision | 一个winner；loser Conflict；current full ref唯一 |
| RB-OWN-05 | conformance | current R1→R2→恶意回指R1 | highest/current CAS拒绝ABA；R1 historical仍可读 |
| RB-OWN-06 | conformance | current index损坏但historical exact存在 | InspectHistorical成功；InspectCurrent Conflict，二者解耦 |
| RB-OWN-07 | conformance | 时间自然跨Expires | `ValidateCurrent`失败；projection revision不增加、digest不变 |
| RB-OWN-08 | conformance | success结果含slice/pointer，caller修改 | 后续Inspect不变，无mutable alias |
| RB-OWN-09 | conformance | active→revoked/superseded terminal projection | 发布新revision；旧projection historical保持sealed，current reader拒绝旧ref |
| RB-OWN-10 | conformance | Create/CAS publish reply丢失 | 只Inspect exact PublishRef；匹配receipt恢复，不重复发布revision |
| RB-OWN-11 | conformance | staged source/binding/projection/receipt失败 | history/highest/current/receipt四者零写 |
| RB-OWN-12 | conformance | Validation Scope association create/CAS/history/highest/current/receipt任一staged失败 | 六者全有全无；失败零泄漏，旧exact association仍可读 |
| RB-REV-11 | whitebox | Artifact在S1/S2间revision漂移 | S2 index mismatch，整批零Verdict |
| RB-REV-12 | whitebox | Environment在S1/S2间lease/revision漂移 | Fail Closed，整批零Verdict |
| RB-REV-13 | whitebox | Validation Scope在S1/S2间coverage digest漂移 | Fail Closed，整批零Verdict |
| RB-REV-14 | whitebox | Context在S1/S2间current index漂移 | live Reader Conflict，整批零Verdict |
| RB-REV-15 | whitebox | 任一Evidence subject/OwnerFact/trust/current漂移 | live Reader Fail Closed，整批零Verdict |
| RB-REV-16 | fault | S2实现错误地重新Resolve并追随新ref | consumer conformance必须检测并失败 |
| RB-REV-17 | fault | current index R1→R2→R1 ABA | S2按saved ref+highest/current检测，零Verdict |
| RB-REV-18 | whitebox | Artifact/Environment/Scope任一Owner Binding在S1/S2间revoke/renew/consumer grant漂移 | Binding full-ref/index检测Fail Closed，零Verdict |
| RB-REV-19 | whitebox | source projection仍旧但Owner Binding已漂移 | 不接受缓存projection；每个source Binding必须独立S1/S2复读 |
| RB-REV-20 | whitebox | Validation Scope association在S1/S2间换Owner、revoke、ABA或TTL crossing | same full Ref Inspect失败；不追随新Owner，零aggregate/Verdict |
| RB-REV-21 | whitebox | route Declaration/Owner/RouteRef不变但ReaderBindingRef或AdapterArtifactDigest被替换 | resolved-route Validate或aggregate逐字段校验`Conflict + BindingDrift`；零Reader调用后的aggregate/Verdict |
| RB-REV-22 | whitebox | Router返回正确Reader但错误nominal resolved-route family，或nil/typed-nil Reader | `PreconditionFailed + OwnerMissing`；零aggregate/Verdict |

## 4. true min TTL与clock

以下每例先让其他输入Expires更晚，再让指定输入成为唯一最短TTL；返回Projection必须精确等于该值，实际点`now >= Expires`必须零Verdict/CAS。

| ID | 唯一最短输入 | Oracle |
|---|---|---|
| RB-TTL-01 | Bundle | aggregate Expires=Bundle.Expires |
| RB-TTL-02 | Request | aggregate Expires=Request.Expires |
| RB-TTL-03 | Target | aggregate Expires=Target.Expires |
| RB-TTL-04 | Context Envelope | aggregate Expires=Envelope.Expires |
| RB-TTL-05 | 单个Context Source | aggregate Expires=该Source.Expires |
| RB-TTL-06 | 第一个Artifact | aggregate Expires=该Artifact projection.Expires |
| RB-TTL-07 | 非首Artifact | aggregate Expires=该Artifact projection.Expires |
| RB-TTL-08 | Environment | aggregate Expires=Environment projection.Expires |
| RB-TTL-09 | Validation Scope | aggregate Expires=Scope projection.Expires |
| RB-TTL-10 | 第一项Evidence | aggregate Expires=该Evidence projection.Expires |
| RB-TTL-11 | 非首Evidence | aggregate Expires=该Evidence projection.Expires |
| RB-TTL-11A | 任一Artifact/Environment/Scope Owner Binding closure | aggregate Expires=该Binding projection.Expires |
| RB-TTL-11B | Validation Scope Owner association | aggregate Expires=association projection.Expires |
| RB-TTL-12 | S1前clock为zero | `InvalidArgument/PreconditionFailed`，零读后写 |
| RB-TTL-13 | Reader内部clock从baseline回拨但仍晚于facts.Checked | ClockRegression，零Verdict/CAS |
| RB-TTL-14 | S2完成后到Verdict CAS前跨TTL | CAS实际点fresh clock拒绝 |

## 5. lost reply、closed errors与root

| ID | 层级 | 场景 | Oracle |
|---|---|---|---|
| RB-FAULT-01 | fault | S1 Resolve丢回包 | 丢弃部分snapshot；可开始新S1，但不声称恢复旧结果 |
| RB-FAULT-02 | fault | S2 exact Inspect丢回包 | 至多一次同subject+Ref重读；不Resolve新ref、不mutation |
| RB-FAULT-03 | fault | caller ctx取消后lost read recovery | 仅宿主批准的bounded detached context；完成后fresh clock/current验证 |
| RB-FAULT-04 | fault | Owner返回Unknown/context.Canceled/DeadlineExceeded | 保持Indeterminate，禁止降NotFound |
| RB-FAULT-05 | fault | ordinary/eventual/retention NotFound | 不认定authoritative absence，不创建Owner projection |
| RB-FAULT-06 | fault | staged Owner read失败 | aggregate projection/Verdict零写，不缓存前序成功projection |
| RB-FAULT-07 | blackbox | cross-tenant Artifact/Environment/Scope/Evidence | Forbidden/Conflict，零信息泄露、零Verdict |
| RB-FAULT-08 | blackbox | Artifact Owner与Kind/SourceContract/Capability不匹配 | Router Fail Closed |
| RB-ROOT-01 | integration | unknown family/kind/version | closed `PreconditionFailed/Forbidden`，无default route |
| RB-ROOT-02 | integration | kind有声明但Reader缺失 | root构造失败，不延迟到首个production请求 |
| RB-ROOT-03 | integration | duplicate alias或同kind换Owner | root构造Conflict |
| RB-ROOT-04 | integration | production root未安装 | Result Grounding不可用，legacy/fixture不得接管 |
| RB-ROOT-05 | import | Router import Sandbox/Continuity实现包 | import boundary test失败 |
| RB-ROOT-06 | constructor | 同key换Owner/contract/capability/Reader，或typed-nil/错误family slice | 构造Conflict/PreconditionFailed；运行期零registry mutation |
| RB-ROOT-07 | constructor | required catalog声明route但整个binding项缺失，或binding多于catalog | constructor `PreconditionFailed + OwnerMissing`；不能延迟到首请求 |
| RB-ROOT-08 | constructor | ComponentID/Capability相同但BindingSet revision、Manifest/Artifact digest不同 | full Owner route不匹配；构造/resolve Fail Closed |
| RB-ROOT-09 | constructor | Go Reader interface实例相同/不同，但sealed ReaderBindingRef冲突或复用两个Route | 只按ReaderBindingRef裁决；不得比较interface/reflect地址 |
| RB-ROOT-10 | contract | 三个Resolve只返回Reader或RouteRef，未返回nominal sealed Proof | compile-shape失败；返回值必须包含Declaration/full Owner、RouteRef、ReaderBindingRef/adapter digest及typed Reader |
| RB-FAULT-09 | fault | recovery timeout为0、负数或大于2s | constructor `InvalidArgument + InvalidReference`，零读 |
| RB-FAULT-10 | fault | configured timeout仍大于当前cut TTL或caller deadline remaining | timeout裁剪到最短；裁剪结果<=0时零retry |
| RB-FAULT-11 | fault | S1 lost reply后第二次S1仍失败，或S2 exact retry再失败 | 全流程最多各一次；返回原Indeterminate/Unavailable，无第三次调用 |
| RB-ERR-01 | conformance | 对每个Validate/Resolve/InspectCurrent/InspectHistorical/Create/CAS/InspectPublish注入表中每种失败 | Category+Reason逐方法与主设计完全相等；NotFound不吞terminal/unknown |
| RB-ERR-02 | conformance | 对全部Derive/Digest/Seal、ResolvedRoute Validate、Grounding Request/StoredFacts/Dependencies、constructor与Inspect注入每种失败 | Category+Reason逐项等于主设计；没有未分类的public error方法 |
| RB-ERR-03 | compile-shape | `ResultBundleCurrentGroundingReaderV2`、具名Request、Dependencies或constructor任一仍是注释占位/弱`any` | 编译shape失败；constructor只接受冻结policy+完整Dependencies结构 |

## 6. legacy与hard stop

| ID | 场景 | Oracle |
|---|---|---|
| RB-LEG-01 | V1 Bundle historical exact Inspect | 可读、字段原样返回 |
| RB-LEG-02 | V1 string Anchor/digest-only Environment/Scope进入production Decide | Fail Closed，零Verdict/current projection |
| RB-LEG-03 | 从V1 digest+latest文件/页面反推V2 exact ref | 禁止；无升级helper |
| RB-LEG-04 | V2 grounding通过后直接写Artifact/Run Outcome/Permit | 分层测试拒绝；只产Review决定输入 |
| RB-LEG-05 | screenshot/video存在但Artifact revision/Environment/Scope不current | `Insufficient Evidence`或closed failure，不能Accept |
| RB-LEG-06 | Bundle被当作有current index、replacement CAS或可替换对象 | 编译/合同中不存在Bundle current-index/replacement CAS Port；V2只create-once atomic append+exact history Inspect，currentness由read cut证明 |

## 7. 未来验证门

| 门 | 命令/证据 | 通过标准 |
|---|---|---|
| targeted ordinary | `go test -count=100 -run 'ResultBundleV2|GroundingCurrent|ArtifactLocator' ./...` | 100次全绿 |
| targeted race | `go test -race -count=20 -run 'ResultBundleV2|GroundingCurrent|ArtifactLocator' ./...` | 20次无race |
| full | `go test ./...` | Review模块全绿 |
| full race | `go test -race ./...` | Review模块无race |
| vet | `go vet ./...` | 零问题 |
| owner conformance | Artifact/Environment/Validation Scope各Owner reusable suite | RB-OWN全绿 |
| integration/system | trusted root + real Owner readers | RB-ROOT及跨Owner负例全绿 |
| mechanical | gofmt、diff-check、import scan、asset links/stale | 全绿 |

当前只冻结oracle，不声称上述Go命令已运行。
