# Workspace Restore V1

状态：**用户已批准完整安全纵切与Host Local真实执行**。

本设计只覆盖`workspace_snapshot`到新Instance隔离Workspace root的恢复。它不恢复旧进程、
Secret、网络会话、设备状态或外部世界，也不覆盖或原地修改source Workspace。

## 1. Owner与边界

| 对象/动作 | 唯一Owner | 非Owner禁止 |
|---|---|---|
| Runtime RestoreAttempt、Identity Reservation、Eligibility、Fence | Runtime | Continuity或Sandbox创建、续期或解释Runtime事实 |
| Restore Intent与跨域顺序 | Application | 直写Runtime、Review、Context或Sandbox领域事实 |
| Review Verdict/Authorization来源 | Review/Runtime既有治理链 | Sandbox从caller布尔值或Plan状态推导Authorization |
| Workspace Snapshot Artifact/Manifest、Restore Stage Attempt/Fact | Sandbox | Continuity捕获、解包或修改Workspace |
| Context Generation/Frame Refresh | Context | Sandbox拼接Context Frame |
| RestorePlan、Manifest/Seal引用与恢复关系 | Continuity | Continuity执行Stage或Activate |

Provider只返回Observation/Receipt和opaque provider attempt；Sandbox Owner必须独立Inspect实际
materialized root，再create-once/CAS领域Fact。Runtime Settlement只引用Sandbox DomainResult exact
ref；Sandbox ApplySettlement不能解释或复制Runtime Outcome。

## 2. Workspace Snapshot Bundle V1

`WorkspaceSnapshotBundleV1`是Host Local Snapshot Content中的strict canonical JSON：

- `contract_version`、`snapshot_id`、`tenant_id`、`source_scope_digest`；
- 排序唯一的`entries`；
- 排序唯一的`excluded`；
- 总字节数、entry-set digest与bundle digest。

Entry闭集：

| kind | 字段 | 落盘语义 |
|---|---|---|
| `directory` | canonical relative path | 创建为`0700` |
| `regular_file` | path、`executable`、length、content digest、content bytes | 非可执行`0600`；可执行`0700` |

禁止absolute、`.`、`..`、反斜杠、重复/父子类型冲突、超过bounds、digest漂移、未知字段、
symlink/submodule/socket/device/FIFO。Capture遇到不支持对象必须生成`excluded` Residual，不得跟随
或静默丢失；Stage只接受已经canonical密封的Bundle。

## 3. 隔离新根与Root Binding

- `RootParent`只由trusted host配置注入；DTO/API/CLI不得携宿主absolute path。
- final root名称从Tenant、RestoreAttempt、TargetInstance/Lease和Bundle digest确定，不接受caller path。
- Provider先在同一RootParent内创建私有temporary root，完整写入、逐项摘要复核、写入exact marker并
  fsync，再以rename提交到从未存在的final root。
- final root已存在时只能Inspect marker与完整文件闭包；exact winner幂等返回，换Bundle/Identity为
  Conflict。不得覆盖、merge、rebase或删除source/其他root。
- 对rename/回包未知只Inspect原provider attempt/final root；不能换Attempt或Target Instance重派。

公共Fact只返回opaque `WorkspaceRootRefV1`，不返回absolute path。只有Sandbox/Agent Host私有
装配可按exact ref解析root。

## 4. 治理与调用顺序

```text
Continuity exact submitted RestorePlan current
  -> Runtime create-once RestoreAttempt + new Instance/high Epoch/new Lease/Fence Reservation
  -> Runtime short-TTL RestoreEligibility current
  -> Application Restore Intent
  -> Runtime Action Admission
  -> Review Verdict/current Authorization
  -> Runtime Permit/Fence + Begin
  -> prepare Enforcement + Sandbox current S1
  -> Provider Prepare/Stage on isolated temporary root
  -> execute Enforcement + Sandbox current S2 at actual execution point
  -> Provider Observation/Receipt
  -> Sandbox independent Inspect + DomainResultFact CAS
  -> Runtime Restore Settlement exact ref
  -> Sandbox ApplySettlement
  -> Context Refresh creates new Generation/Frame
  -> Runtime/Application Activation binds exact Target Instance/Lease/Fence + WorkspaceRoot + Context refs
```

任何门缺失、过期、漂移或typed-nil都必须在Provider前Fail Closed。Eligibility不是Admission、
Authorization、Permit或Begin。Activation必须在Workspace Stage与Context Refresh Settlement之后；
旧Instance不能重新获得active权。

## 5. Unknown、Residual与硬反例

- `UnknownOutcome`后只Inspect原Runtime/Sandbox/provider attempt；不得换identity重派。
- Partial Checkpoint、Residual非空、Snapshot Artifact非`available`、content/marker/file digest漂移均不得
  Activate。
- symlink、submodule、socket、device、FIFO、Secret mount、外部network/process/session/device状态
  只进入declared exclusion/Residual；首版不恢复。
- 不宣称邮件、交易、网络请求、远端DB写或任何外部世界回滚。
- legacy `RestoreRequest`、Checkpoint phase Evidence/Settlement、`activation_attempt`或transport kind
  不能包装成Restore Stage资格。

## 6. 验收

- unit/whitebox：canonical、bounds、clone/no-alias、path、parent closure、mode、digest。
- blackbox：capture -> encrypted Content Store -> isolated stage -> independent Inspect。
- fault：lost put/stage/rename reply、partial temp、tamper、disk/full-like write error、stale current。
- concurrency：same identity exact replay；64路different-content单winner；no-ABA。
- safety：source root零修改、pre-existing target零覆盖、symlink/path escape零写、absolute path零DTO。
- `target100`、`race20`、full ordinary/race/vet、gofmt/import/diff/links/XML全部通过。

