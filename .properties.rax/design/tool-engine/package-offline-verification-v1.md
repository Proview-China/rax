# Tool Package Offline Verification与Admission V1

## 1. 结论与状态

- 业务目标：整理和组装现有Tool/MCP/Skill/App/WASM生态，不自造签名、分发或证明协议；
- 候选组合：OCI Image Manifest/Descriptor承载内容寻址的Package Artifact，Sigstore Bundle承载离线验签材料，in-toto Statement V1把证明绑定到Artifact subject；
- 用户已确认首个完整边界必须在同一CAS中复读Verification、Package current、Trust Policy与Artifact exact后才Admission；不接受只实现Verify或暂缓Package；
- V1仍不做Registry网络Fetch、Install、Enable、透明日志在线查询或production Artifact Store；
- 状态：Runtime public `SupplyChainArtifact/Trust V1` neutral nominal与Readers已经落盘，Tool
  Owner-local离线Verify、immutable Observation/Fact/current、verification-aware强Admission及
  SDK/API/CLI exact入口已经实现。官方Sigstore Go + in-toto离线key-bundle正向Conformance、
  tamper/TTL/clock/lost-reply/64并发反例、targeted ordinary×100、race×20、模块full
  ordinary/race/vet及Runtime Supply Chain ports门均已通过，本切片状态为
  `implementation_software_test_yes`。Fetch/Install/Enable、production Artifact/Trust backend、
  在线Trust freshness与composition root继续NO-GO。

标准来源：

- [OCI Image Manifest](https://github.com/opencontainers/image-spec/blob/main/manifest.md)允许通过`artifactType`、content-addressed Descriptor和`subject`组织非容器Artifact；
- [OCI Distribution Referrers](https://github.com/opencontainers/distribution-spec/blob/main/spec.md)属于后续Fetch/Discovery，不进入本离线切片；
- [Sigstore Go](https://github.com/sigstore/sigstore-go)的Bundle包含验签所需签名与Verification Material，并支持离线验证；
- [in-toto Statement V1](https://github.com/in-toto/attestation/blob/main/spec/v1/statement.md)用`subject.digest`与`predicateType`绑定immutable Artifact和证明类型。

## 2. Owner与非Owner

| 对象 | Owner | 非Owner限制 |
|---|---|---|
| Package Manifest、Verification Fact/Current、Registry状态 | Tool Engine | 不签发组织Trust Policy，不自动Enable |
| Trust Root、允许的Signer Identity、透明日志/时间戳要求 | 组织Policy/Trust Owner | 不写Tool Registry事实 |
| OCI/Sigstore/in-toto语法与密码学验证 | 上游标准库/注入Verifier Adapter | 回包只是Verification Observation |
| Fetch、Credential、Network、Budget、Review、Permit | Runtime/Application及对应Owner | Tool不得用Verify绕过外部Effect治理 |
| Artifact/Bundle bytes | 外部State Plane content-addressed Store | Tool合同不预选数据库、Registry或Blob实现 |

`verified`只表示“给定exact bytes在给定current Trust Policy下通过验证”。它不表示已注册、已启用、可进入Agent Plan或可执行其中的Effect。

## 3. 现有V1兼容边界

`ToolPackageManifest.Signatures []Digest`只能绑定声明中列出的摘要，不能表达签名算法、Signer、证书、Transparency Log、Trust Root或Verification Policy，禁止把它包装成正式验签结果。

因此保持以下不变量：

1. `ToolPackageManifest V1`、Registry Record、Package Assembly digest不变；
2. 不给`Signatures`添加隐式Sigstore或公钥语义；
3. 新的`ToolPackageVerificationFactV1`通过exact `Package Ref + Artifact Ref + Bundle Ref + Statement Ref + TrustPolicy Current Ref`关联V1；
4. Registry从`submitted`到`admitted`的强写口必须在同一CAS消费fresh Verification Current、Package current、Trust Policy与Artifact exact；Verify本身不执行transition，V1不自动active/enable。

## 4. Additive强类型合同

Tool合同落点为`contract/package_verification_v1.go`。跨Owner Artifact/Trust类型直接复用
Runtime public `ports/supply_chain_artifact_trust_v1.go`，Tool没有复制同形类型。

### 4.1 Runtime-neutral Artifact与Trust Port Delta

```go
// type owner: Runtime public ports; semantic/repository owner: Artifact Owner.
type SupplyChainArtifactContentRefV1 struct {
    ContractVersion string
    MediaType string
    Digest core.Digest
    Size uint64
}

type SupplyChainArtifactExactReaderV1 interface {
    OpenExactSupplyChainArtifactV1(context.Context, SupplyChainArtifactContentRefV1) (io.ReadCloser, error)
}

type SupplyChainTrustMaterialRefV1 struct {
    ContractVersion string
    ID string
    Revision core.Revision
    Digest core.Digest
}

type SupplyChainTrustPolicyRefV1 struct {
    ContractVersion string
    ID string
    Revision core.Revision
    Digest core.Digest
}

type SupplyChainTrustPolicyCurrentRefV1 struct {
    ContractVersion string
    ID string
    Revision core.Revision
    Digest core.Digest
}

type SupplyChainTrustPolicyCurrentProjectionV1 struct {
    ContractVersion string
    Ref SupplyChainTrustPolicyCurrentRefV1
    Policy SupplyChainTrustPolicyRefV1
    TrustedRoot SupplyChainTrustMaterialRefV1
    IdentityPolicyDigest core.Digest
    PredicatePolicyDigest core.Digest
    TransparencyPolicyDigest core.Digest
    TimestampPolicyDigest core.Digest
    MaxPackageArtifactBytes uint64
    MaxSigstoreBundleBytes uint64
    MaxInTotoStatementBytes uint64
    MaxTrustMaterialBytes uint64
    CheckedUnixNano int64
    ExpiresUnixNano int64
    ProjectionDigest core.Digest
}

type SupplyChainTrustPolicyCurrentReaderV1 interface {
    InspectCurrentSupplyChainTrustPolicyV1(context.Context, SupplyChainTrustPolicyCurrentRefV1) (SupplyChainTrustPolicyCurrentProjectionV1, error)
}

type SupplyChainTrustMaterialExactReaderV1 interface {
    OpenExactSupplyChainTrustMaterialV1(context.Context, SupplyChainTrustMaterialRefV1) (io.ReadCloser, error)
}

type SupplyChainTrustPolicyDocumentRefV1 struct {
    ContractVersion string
    ID string
    Revision core.Revision
    MediaType string
    Digest core.Digest
    Size uint64
}

type SupplyChainTrustPolicyDocumentExactReaderV1 interface {
    OpenExactSupplyChainTrustPolicyDocumentV1(context.Context, SupplyChainTrustPolicyDocumentRefV1) (io.ReadCloser, error)
}
```

- `OpenExact*`是调用形状候选，不把`io.ReadCloser`下沉到领域Fact；Reader实现必须边读边校验
  exact Digest/Size并在close前完成，提前EOF、额外bytes或digest漂移均失败；
- Reader只读既有State Plane内容，不联网。URL/tag/latest/name不进入Request；
- typed-nil Reader/stream、nil/canceled context、close/digest失败均不得产生Observation或Fact；
- Trust Policy current只含exact Root Ref与版本化Policy摘要。Tool不拿Trust写口，也不把
  `Allowed=true`布尔值当Policy。
- `SupplyChainTrustPolicyDocumentRefV1`是Tool Sigstore adapter实际解释的exact、versioned、
  bounded政策正文；Runtime只拥有neutral坐标，不解释Sigstore policy schema。证书模式要求
  certificate identities、timestamp与transparency；key模式要求exact PEM root、
  `none_for_key`且不得伪造timestamp/SCT/tlog。
- `SupplyChainTrustPolicyRefV1`是历史政策事实，`SupplyChainTrustPolicyCurrentRefV1`
  是该政策的immutable current lease，两者禁止type-pun。Current Ref的ID/Revision由
  Trust/Policy Owner签发；Digest与ProjectionDigest都是同一canonical projection body的摘要，
  计算时同时排除`Ref.Digest`与`ProjectionDigest`，防止digest回流。同一Current Ref必须
  返回全字段immutable Projection，不得在Inspect时刷新Checked/Expires。
- V1所有content digest固定为`sha256`并映射`core.Digest`；其他OCI digest algorithm必须由
  successor合同显式支持，不做字符串type-pun。Artifact/Bundle/Statement三个非零size上限来自current Trust Policy，
  Trust Material上限也来自同一Projection；本合同不硬编码任意生产容量。

### 4.2 Tool-owned Package Artifact Binding

```go
type ToolPackageArtifactBindingV1 struct {
    ContractVersion string
    Package ObjectRef
    OCIManifest runtimeports.SupplyChainArtifactContentRefV1
    PackageArtifact runtimeports.SupplyChainArtifactContentRefV1
    SigstoreBundle runtimeports.SupplyChainArtifactContentRefV1
    InTotoStatement runtimeports.SupplyChainArtifactContentRefV1
    ArtifactType string
    BindingDigest core.Digest
}
```

- `PackageArtifact.Digest == ToolPackageManifest.ArtifactDigest`；既有字段明确绑定Package payload，
  不偷偷改成OCI wrapper Manifest digest；
- OCI Manifest exact引用Package Artifact，并以标准`artifactType`声明类型；未知artifact type可被
  State Plane保存，但Tool Verify只接受版本化Allowlist；
- Sigstore Bundle与in-toto Statement均为独立exact Content Ref。Statement `_type`固定V1，
  subject基数固定1且digest exact等于Package Artifact；
- Sigstore Bundle中被验证的payload digest必须exact等于`InTotoStatement.Digest`，禁止分别验证
  一个合法Bundle和另一份合法但无关Statement；
- Binding canonical domain=`praxis.tool-mcp.package-verification`、version=`1.0.0`、
  discriminator=`ToolPackageArtifactBindingV1`，body排除`BindingDigest`；所有字符串、Ref和重复
  Package字段在`Validate/Seal/ValidateAgainst`逐字段硬门。

### 4.3 Tool Package Registry current

现有`registry.Record`没有Record digest、current TTL或Package Verification关联。新增Tool-owned只读
projection候选：

```go
type ToolPackageRegistryCurrentRefV1 struct {
    ContractVersion string
    ID string
    Revision core.Revision
    Digest core.Digest
}
type ToolPackageRegistryRecordSourceV1 struct {
    Kind string
    ID string
    ObjectRevision core.Revision
    ObjectDigest core.Digest
    State string
    RegistryRevision core.Revision
    UpdatedUnixNano int64
    Digest core.Digest
}
type ToolPackageRegistryCurrentProjectionV1 struct {
    ContractVersion string
    Ref ToolPackageRegistryCurrentRefV1
    Source ToolPackageRegistryRecordSourceV1
    Package ObjectRef
    Manifest ToolPackageManifest
    State string
    RegistryRevision core.Revision
    CheckedUnixNano int64
    ExpiresUnixNano int64
    ProjectionDigest core.Digest
}
type ToolPackageRegistryCurrentReaderV1 interface {
    InspectCurrentToolPackageRegistryV1(context.Context, ToolPackageRegistryCurrentRefV1) (ToolPackageRegistryCurrentProjectionV1, error)
}
```

Ref ID由Package ID派生；Revision等于RegistryRevision；Ref Digest是source Record canonical
digest，ProjectionDigest另算完整fresh projection。`Source/Package/Manifest/State/RegistryRevision`
重复字段必须逐字段exact，`Manifest.ID/Revision/Digest == Package`，
`Manifest.ArtifactDigest == ArtifactBinding.PackageArtifact.Digest`。Verify只接受
`submitted|admitted`来源；`deprecated|revoked`永远Fail Closed。该Projection是Tool Registry事实，
不伪装Runtime Authority。

source Record digest固定为canonical domain=`praxis.tool-mcp.registry`、version=`1.0.0`、
discriminator=`ToolPackageRegistryRecordSourceV1`，body逐字段包含live `registry.Record`的
`Kind/ID/ObjectRevision/ObjectDigest/State/RegistryRevision/UpdatedUnixNano`。Ref不使用含fresh
`Checked/Expires`的ProjectionDigest。

Package Registry current Reader由Tool Registry Owner实现，只允许按exact Ref读；禁止
`latest/name/state`弱查询。Current lease不硬编码15秒或其他任意生产上限；expiry取Registry
source真实窗口、Trust Policy、Artifact Reader能力与caller requested deadline的共同最早上界。
缺少任何真实上界时Fail Closed。

### 4.4 Verification请求、Observation与权威Fact

```go
type ToolPackageVerificationSubjectV1 struct {
    ContractVersion string
    PackageRegistry ToolPackageRegistryCurrentRefV1
    ArtifactBinding ToolPackageArtifactBindingV1
    TrustPolicy runtimeports.SupplyChainTrustPolicyRefV1
    VerifierProfile runtimeports.NamespacedNameV2
}

type ToolPackageVerifyRequestV1 struct {
    ContractVersion string
    Subject ToolPackageVerificationSubjectV1
    TrustPolicyCurrent runtimeports.SupplyChainTrustPolicyCurrentRefV1
    RequestedExpiresUnixNano int64
}

type ToolPackageVerificationObservationRefV1 struct {
    ContractVersion string
    ID string
    Revision core.Revision
    Digest core.Digest
}
type ToolPackageVerificationObservationEnsureRequestV1 struct {
    ContractVersion string
    Subject ToolPackageVerificationSubjectV1
    TrustPolicyCurrent runtimeports.SupplyChainTrustPolicyCurrentRefV1
    TrustedRoot runtimeports.SupplyChainTrustMaterialRefV1
    IdentityPolicyDigest core.Digest
    PredicatePolicyDigest core.Digest
    TransparencyPolicyDigest core.Digest
    TimestampPolicyDigest core.Digest
    SignerIdentityDigest core.Digest
    PredicateType string
    VerifierConformance runtimeports.NamespacedNameV2
}
type ToolPackageVerificationObservationV1 struct {
    ContractVersion string
    Ref ToolPackageVerificationObservationRefV1
    Request ToolPackageVerificationObservationEnsureRequestV1
    ObservedUnixNano int64
}
```

Observation Ref stable ID从`Subject + TrustPolicyCurrent Ref` canonical digest派生，revision固定1；
caller deadline/RequestedExpires、Repository clock和fresh Checked/Expires均不进入ID。Ref.Digest是完整
Observation canonical digest，不再另设循环`ObservationDigest`。Verifier Profile进入Subject，避免同一
stable key在不同验证规则下产生不同结果。Signer只保存上游验证器输出的canonical identity摘要，
不保存证书原文、邮箱或Secret。

`ObservedUnixNano`不由Verifier/caller提供；唯一Repository在create-once线性化点用Owner clock写入。
并发loser只比较Ensure Request中的稳定语义字段，完全一致时返回winner，不用各自
fresh clock与winner整体比较。Signer/Predicate/Policy任一改变均Conflict。

注入Verifier只产生Observation。Tool先create-once保存Observation；Tool Owner在S1/S2复读exact
Package Registry、Artifact/Bundle/Statement bytes及Trust Policy current后，独立Validate并create-once：

```go
type ToolPackageVerificationFactRefV1 struct {
    ContractVersion string
    ID string
    Revision core.Revision
    Digest core.Digest
}
type ToolPackageVerificationFactEnsureRequestV1 struct {
    ContractVersion string
    Subject ToolPackageVerificationSubjectV1
    Observation ToolPackageVerificationObservationRefV1
}
type ToolPackageVerificationFactV1 struct {
    ContractVersion string
    Ref ToolPackageVerificationFactRefV1
    Package ObjectRef
    PackageRegistry ToolPackageRegistryCurrentRefV1
    ArtifactBindingDigest core.Digest
    TrustPolicy runtimeports.SupplyChainTrustPolicyRefV1
    Observation ToolPackageVerificationObservationRefV1
    SignerIdentityDigest core.Digest
    PredicateType string
    VerifierConformance runtimeports.NamespacedNameV2
    VerifiedUnixNano int64
}

type ToolPackageVerificationCurrentRefV1 struct {
    ContractVersion string
    ID string
    Revision core.Revision
    Digest core.Digest
}
type ToolPackageVerificationCurrentIssuanceV1 struct {
    ContractVersion string
    Fact ToolPackageVerificationFactRefV1
    PackageRegistry ToolPackageRegistryCurrentRefV1
    TrustPolicyCurrent runtimeports.SupplyChainTrustPolicyCurrentRefV1
    RequestedExpiresUnixNano int64
}
type ToolPackageVerificationCurrentProjectionV1 struct {
    ContractVersion string
    Ref ToolPackageVerificationCurrentRefV1
    Issuance ToolPackageVerificationCurrentIssuanceV1
    Fact ToolPackageVerificationFactV1
    CurrentPackageRegistry ToolPackageRegistryCurrentProjectionV1
    TrustPolicy runtimeports.SupplyChainTrustPolicyCurrentProjectionV1
    CheckedUnixNano int64
    ExpiresUnixNano int64
    ProjectionDigest core.Digest
}
```

Fact Ref ID从Observation Ref与Package Ref派生，revision固定1；Ref.Digest即完整Fact canonical digest，
不再另设`FactDigest`。`VerifiedUnixNano`与Observation一样只由Repository Owner clock在单赢家
create-once时写入；Fact是历史权威事实。执行Registry transition前另签发短期Current，复读Fact、
当前Package Registry、Artifact/Material exact与同一历史Policy的fresh Trust Policy current；Fact来源Registry revision可以是
更早的`submitted|admitted`，但当前Registry必须绑定同一Package Object且不是deprecated/revoked。
expiry取当前Package Registry、
Trust Policy、caller deadline和RequestedExpires的最小真实上界，不硬编码任意TTL或production SLA。

`RequestedExpiresUnixNano`必须大于当前S1时间且只能缩短；等于0或已过期Fail Closed。它只属于
Current issuance identity，不进入Observation/Fact identity。Current ID从完整Issuance canonical
digest派生，revision固定1；Ref.Digest与ProjectionDigest均由排除二者的同一immutable
Projection body计算。同一Fact可在相同exact历史Trust Policy仍有fresh current lease时，
针对更新后的同Package Registry state签发新Current；不得覆盖Fact或用旧Registry current。

### 4.5 精确方法、canonical与Repository语义

Tool合同的canonical domain统一为`praxis.tool-mcp.package-verification`，version=`1.0.0`；
discriminator分别固定为类型名。每个`ComputeDigest/Seal`都排除本层自身digest字段，
不排除下层exact Ref。所有Seal输入和Repository返回都deep-copy，nil slice与empty slice
按合同固定canonical，调用方修改不得回写Store。

```go
func (ToolPackageArtifactBindingV1) Validate() error
func (ToolPackageArtifactBindingV1) ValidateAgainst(ToolPackageManifest) error
func SealToolPackageArtifactBindingV1(ToolPackageArtifactBindingV1) (ToolPackageArtifactBindingV1, error)

func (ToolPackageRegistryCurrentRefV1) Validate() error
func (ToolPackageRegistryCurrentProjectionV1) Validate() error
func (ToolPackageRegistryCurrentProjectionV1) ValidateCurrent(ToolPackageRegistryCurrentRefV1, time.Time) error

func (ToolPackageVerificationSubjectV1) Validate() error
func (ToolPackageVerifyRequestV1) ValidateCurrent(time.Time) error
func (ToolPackageVerificationObservationEnsureRequestV1) Validate() error
func (ToolPackageVerificationObservationV1) Validate() error
func (ToolPackageVerificationFactEnsureRequestV1) Validate() error
func (ToolPackageVerificationFactV1) Validate() error
func (ToolPackageVerificationCurrentIssuanceV1) ValidateCurrent(time.Time) error
func (ToolPackageVerificationCurrentProjectionV1) Validate() error
func (ToolPackageVerificationCurrentProjectionV1) ValidateCurrent(ToolPackageVerificationCurrentRefV1, time.Time) error

type ToolPackageVerificationRepositoryV1 interface {
    EnsureToolPackageVerificationObservationV1(context.Context, ToolPackageVerificationObservationEnsureRequestV1) (ToolPackageVerificationObservationV1, error)
    InspectToolPackageVerificationObservationBySubjectV1(context.Context, ToolPackageVerificationSubjectV1, runtimeports.SupplyChainTrustPolicyCurrentRefV1) (ToolPackageVerificationObservationV1, error)
    InspectExactToolPackageVerificationObservationV1(context.Context, ToolPackageVerificationObservationRefV1) (ToolPackageVerificationObservationV1, error)
    EnsureToolPackageVerificationFactV1(context.Context, ToolPackageVerificationFactEnsureRequestV1) (ToolPackageVerificationFactV1, error)
    InspectToolPackageVerificationFactByObservationV1(context.Context, ToolPackageVerificationObservationRefV1) (ToolPackageVerificationFactV1, error)
    InspectExactToolPackageVerificationFactV1(context.Context, ToolPackageVerificationFactRefV1) (ToolPackageVerificationFactV1, error)
}

type ToolPackageVerificationCurrentResolverV1 interface {
    ResolveCurrentToolPackageVerificationV1(context.Context, ToolPackageVerificationCurrentIssuanceV1) (ToolPackageVerificationCurrentProjectionV1, error)
    InspectToolPackageVerificationCurrentByIssuanceV1(context.Context, ToolPackageVerificationCurrentIssuanceV1) (ToolPackageVerificationCurrentProjectionV1, error)
    InspectCurrentToolPackageVerificationV1(context.Context, ToolPackageVerificationCurrentRefV1) (ToolPackageVerificationCurrentProjectionV1, error)
}
```

`Validate`顺序固定为intrinsic shape -> nested exact refs -> repeated-field exact -> canonical digest；
`ValidateCurrent`再执行fresh time、clock rollback、TTL与expected exact Ref。Ensure必须先Validate
request，按stable subject派生ID，先Inspect；只有权威NotFound才在唯一Store中create-once。
same ID/same稳定内容返回winner；same ID换Material/Policy/Signer/Predicate返回Conflict。
Current Resolve也必须先用完整Issuance派生ID并`InspectByIssuance`，只有权威NotFound才
做S1/S2与create-once；lost reply或进程重启不需要先重做验签即可恢复winner。
`Unavailable|Indeterminate`不得当NotFound；typed-nil依赖、nil context在进入Reader/lock前拒绝；
`context.Canceled|DeadlineExceeded`保留sentinel，不伪造Runtime error category。

## 5. 私有材料与Verifier seam

候选实现落点：

- `contract/package_verification_v1.go`：仅Tool nominal、canonical、Validate/Seal/Clone；只依赖Runtime public neutral值类型；
- `packageverify/material_reader_v1.go`：消费Runtime-neutral content-addressed exact Reader，只按Digest+Size流式读取，不联网；
- `packageverify/sigstore_bundle_verifier_v1.go`：组装官方Sigstore Go verifier，不复制Bundle/证书/Transparency nominal；
- `packageverify/repository_v1.go`：唯一Observation/Fact history与current projection Repository；
- `packageverify/registry_current_v1.go`：将Tool Registry Record无损投影为Package Registry current；
- `sdk/package_verify_v1.go`：只接已Seal Verify Request和公开Reader，不接受raw URL、tag、public key bytes、Credential或Registry client；
- `internal/testkit/package_verify_v1.go`：固定离线Bundle/Statement/Artifact fixture。

生产实现必须注入Trust Policy Reader和content-addressed Store；测试Fake不宣称production trust、backend或SLA。

### 5.1 官方Go生态组装决议

| 能力 | 首批候选 | 使用边界 |
|---|---|---|
| OCI Manifest/Descriptor | `github.com/opencontainers/image-spec/specs-go/v1`，module `v1.1.1` | 只解析已读入的exact bytes，不调Distribution/Registry网络API |
| Sigstore Bundle/Verify | `github.com/sigstore/sigstore-go/pkg/{bundle,root,verify}`，module `v1.1.4` | `bundle.Bundle.UnmarshalJSON` + `verify.NewVerifier` + `verify.NewPolicy`；禁止`LoadJSONFromPath`、TUF network fetch与`WithoutArtifactUnsafe` |
| in-toto Statement | `github.com/in-toto/attestation/go/v1` | 调`Statement.Validate`后仍必须额外强制`_type=v1`、subject基数1、sha256及Package Artifact exact digest |

`sigstore-go v1.1.4`当前在自身`go.mod`选择`in-toto/attestation v1.1.2`，首批与其已测
组合保持一致；不为了追求单个依赖的最新标签而未验证地强升。上游版本必须在
Tool模块`go.mod`精确固定，不使用`latest`/分支/未审commit；升级需重跑Bundle、identity、
Transparency、timestamp、Statement subject及离线无网络Conformance。

## 6. 时序、恢复与错误

```text
Verify request
  -> Validate stable Subject
  -> S1 exact Package Registry current
  -> S1 current Trust Policy
  -> exact Artifact/Bundle/Statement streams + digest/size
  -> upstream Sigstore/in-toto verification -> Observation Ensure Request
  -> derive ID + Inspect; NotFound才create-once Observation
  -> S2 same exact Package Registry + same exact material refs + same Trust Policy ref/current
  -> Tool Owner Validate Observation -> derive ID + Inspect -> create-once Verification Fact
  -> caller提交exact Package Registry current + fresh same-Policy current
  -> Resolve/Inspect immutable Verification Current
```

- Verify本身无外部Effect；material Reader若要网络Fetch，必须由独立`praxis.tool/package-fetch` Operation先完成；
- Observation/Fact create回包丢失后只按stable request派生ID并Inspect，不重新Fetch；若尚无
  Observation，重新验证同一exact本地bytes是安全纯计算，不得换Material或Policy；
- `NotFound`只表示本地exact material/fact不存在，不证明上游Artifact不存在；
- `Unavailable`、Trust Policy过期、clock rollback、digest/size/identity/predicate漂移均Fail Closed；
- same Package/Artifact/Policy/Material幂等；不同RequestedExpires仍复用同一Observation/Fact，只缩短各自
  current lease；same Fact ID换任一稳定内容Conflict；64同canonical单Fact，异canonical可并行。

## 7. SDK/API/CLI边界

- SDK `PackageVerificationV1`已经只消费sealed exact refs和已存在材料，提供Verify、
  Observation/Fact/current Inspect与强Admission；不接受URL/tag/latest、raw key、信任布尔值或
  `--insecure`；
- transport-neutral API已经提供Observation/Fact/current exact双读，不决定HTTP/gRPC；
- 可嵌入CLI Runner已经提供`package verify --request-json=<sealed exact request>`；严格拒绝
  unknown/trailing JSON，且不会串联Admission、Fetch或Enable；
- `package install|fetch|enable|revoke`仍unsupported，不能由Verify命令串联隐藏执行。

## 8. Package Registry Admission P0与跨OwnerPort Delta

live generic `registry.Registry.Transition("package", ..., admitted|active)`已经Fail Closed；
现有Package assembly旧测试通过独立test-only投影fixture隔离，不构成生产绕过。强Admission只允许
`packageverify.ServiceV1 -> VerifiedPackageAdmissionRegistryV1.AdmitVerifiedPackageV1`，在同一
Registry锁/CAS内复读Package current、Verification Fact/current、Trust Policy与Artifact exact。
它只推进`submitted -> admitted`，不会自动active/enable。

已实现的强类型Tool Owner写口为：

```go
TransitionVerifiedPackageV1(ctx, packageCurrent, verificationCurrent, expectedRegistryRevision, target)
```

它在同一Registry锁/事务内复读Package exact、Verification Fact/Current、Trust Policy ref、Artifact
Binding与expected Registry revision，然后CAS transition。generic `Transition("package", admitted|active)`
必须Fail Closed或降为不导出的fixture helper；不得先transition再异步附加Verification。

已落盘公共边界与仍保留的跨Owner责任：

1. Runtime public ports已经提供neutral `SupplyChainArtifactContentRefV1`、Artifact exact Reader、
   `SupplyChainTrustPolicyDocumentRefV1`/exact Reader、`SupplyChainTrustPolicyCurrent*V1`与
   Trust Material exact Reader的唯一nominal；
2. Artifact与Trust/Policy Owner分别实现上述Reader；Runtime不拥有其语义事实；
3. Admin Application为未来Fetch/Register/Enable定义独立Operation，不把多个Effect折成Verify；
4. Runtime使用`OperationScopeAdminV3`分别治理`package-fetch/register/revoke`；离线Verify不伪装Effect；
5. Transparency/Revocation freshness若无法由离线Bundle+current Trust Policy证明，保持Residual/NO-GO，不在线静默补查。

### 8.1 PD-TM-PKG-01：Artifact/Trust中立只读Port

| 字段 | 冻结内容 |
|---|---|
| 用例 | 对已在State Plane的exact OCI Artifact、Sigstore Bundle、in-toto Statement与Trust Root做离线Verify |
| 语义Owner | Artifact Owner拥有bytes；Trust/Policy Owner拥有Policy/Root；Runtime ports只拥有neutral nominal |
| 输入 | exact content Ref、exact Trust Material Ref、exact immutable Trust Policy Current Ref |
| 输出 | bounded `io.ReadCloser`或全字段immutable Trust Policy Current Projection |
| 不变量 | 只支持sha256；Ref/size/media type exact；Policy historical/current分层；禁止URL/tag/latest/raw key |
| Effect/Recovery | Reader本身不联网；read/close/digest失败Fail Closed；lost reply重试同exact Ref |
| 反例 | typed-nil、短读/多读、close error、same Ref换bytes/Policy、current lease刷新原Projection |
| 兼容 | additive；不改legacy Tool/MCP/Action Port；Tool直接导入Runtime public nominal |

### 8.2 PD-TM-PKG-02：Verification-aware Package transition

| 字段 | 冻结内容 |
|---|---|
| 用例 | 将Package从`submitted -> admitted -> active`时原子消费fresh Verification Current |
| 语义Owner | Tool Registry Owner；Runtime/Policy/Artifact Owner不写Registry状态 |
| 输入 | exact Package Current、Verification Current、expected Registry Revision、target |
| 输出 | successor Package Registry Record/Current exact Ref |
| 不变量 | 同一Package Object/Artifact/Policy；current fresh；Tool dependencies达到目标state；同锁/同事务CAS |
| Effect/Recovery | 只修改Tool Registry领域事实；CAS丢回包后Inspect exact Registry revision，不重放另一transition |
| 反例 | generic Transition无Verification通过；先transition后附Fact；过期/换Policy/换Artifact/旧Registry current |
| 兼容 | additive strong write Port；legacy generic Package admission必须Fail Closed或限test fixture；Capability/Tool transition不受影响 |

## 9. 硬反例与测试矩阵

- V1 `Signatures []Digest`直接当验签通过；
- OCI tag/latest或Artifact name替代exact digest；
- Bundle签名合法但in-toto subject不是Package Artifact；
- Package `ArtifactDigest`、OCI Descriptor、Statement subject三者任一漂移；
- Signer合法但不在current Policy，或Policy在S1/S2间换revision；
- Bundle缺必需Transparency Log/时间戳证明仍通过；
- unknown predicate、重复subject、额外unbound Artifact、非canonical digest/size；
- 超限材料被截断后验签，或只保存摘要而丢失可复核bytes；
- Verify成功直接把Registry变active、进入Plan或获得运行时Authority；
- lost reply后触发Fetch/重新验远端，而不是Inspect原Fact；
- typed-nil Reader/Verifier、nil/canceled context、clock rollback、TTL crossing产生Fact；
- 64同canonical产生多个Fact，same ID换Material/Policy/Signer不Conflict；
- Tool importTrust/Artifact/Application实现或Runtime kernel/fakes/internal。
- generic Registry Transition在无Verification current时把Package推进admitted/active；
- Observation未先持久即创建Fact，或Fact/current使用重复/循环digest字段；
- stream提前EOF、额外bytes、read/close error仍验签成功；
- Package Registry Ref把ProjectionDigest误作source Record digest，或S1/S2换RegistryRevision；
- RequestedExpires/fresh Checked进入Observation/Fact stable ID，导致同一Artifact并发产生多个Fact；
- Fact绑定submitted Registry revision后无法对同一Package admitted current重新签发Current，或反向用
  旧submitted current推进active；

## 10. GO/NO-GO

Runtime-neutral Trust Policy Document/Current、Artifact与Trust Material public nominal/Readers
已经落盘；Tool Owner-local离线Verify与verification-aware Admission同一CAS已经实现。targeted
ordinary×100、race×20、模块full ordinary/race、vet、Runtime Supply Chain ports门及官方
Sigstore/in-toto正向Conformance均通过，当前结论是`implementation_software_test_yes`，不是
production GO。Fetch/Install/Enable、production Registry/Artifact backend、在线透明日志
freshness/撤回与市场信任仍单独NO-GO。
