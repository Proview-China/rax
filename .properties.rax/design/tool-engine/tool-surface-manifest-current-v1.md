# ToolSurfaceManifestCurrent V1

状态：**C2 Tool-owned Repository P0 slice资产已冻结；Go未实施，独立设计复审未完成。它不解锁PD-TM-04 P4、Harness M2、system或production。**

测试合同见[测试矩阵](tool-surface-manifest-current-v1-test-matrix.md)，实施顺序见[Repository P0计划](../../plan/tool-mcp/tool-surface-manifest-current-v1.md)。

## 1. Owner与边界

- Tool Owner拥有`ToolSurfaceManifestCurrent*V1` nominal、canonical、唯一Repository、current index和语义。
- 公共contract精确拆成只读`ToolSurfaceManifestCurrentReaderV1`与嵌入它的`ToolSurfaceManifestCurrentRepositoryV1`。
- Harness只import `ExecutionRuntime/tool-mcp/contract`；M2构造器只接Reader静态类型，不能看到Repository method set，任何M2读取路径`Ensure`调用数必须为0。
- Repository只确证Tool Surface Manifest current，不拥有Registry、Model、Harness、Runtime或Provider事实，也不授Authority/Review/Fence/执行权。
- Manifest中的`RegistrySnapshotDigest`只作跨Owner关联坐标；Registry Owner exact读取由M2/组合层的独立公共Reader完成，不回显进本Projection，不允许裸digest查latest。
- 本slice不创建SurfaceInvocationBinding、ActionCandidate、BindingV2、Model ACK、Harness Gate或production root。

依赖方向：

```text
runtime/core + runtime/ports public values
                    ↑
tool-mcp/contract ← tool-mcp/surface concrete Repository
                    ↑
       Harness M2 narrow Reader only
```

Tool不import Harness；Harness不import Tool实现，因此无Harness↔Tool SCC。

## 2. 公共contract

```go
const ToolSurfaceManifestCurrentContractVersionV1 = "praxis.tool-mcp.surface-manifest-current/v1"

type ToolSurfaceManifestCurrentRefV1 struct {
    ContractVersion string        `json:"contract_version"`
    ID              string        `json:"id"`
    Revision        core.Revision `json:"revision"`
    Digest          core.Digest   `json:"digest"`
}

type ToolSurfaceManifestCurrentEnsureRequestV1 struct {
    ContractVersion string              `json:"contract_version"`
    Manifest        ToolSurfaceManifest `json:"manifest"`
    ExpectedCurrent ToolSurfaceManifestCurrentRefV1 `json:"expected_current"`
}

type ToolSurfaceManifestCurrentProjectionV1 struct {
    ContractVersion string                          `json:"contract_version"`
    Ref             ToolSurfaceManifestCurrentRefV1 `json:"ref"`
    Manifest        ToolSurfaceManifest             `json:"manifest"`
    Owner           core.OwnerRef                   `json:"owner"`
    CheckedUnixNano int64                           `json:"checked_unix_nano"`
    ExpiresUnixNano int64                           `json:"expires_unix_nano"`
    ProjectionDigest core.Digest                     `json:"projection_digest"`
}

type ToolSurfaceManifestCurrentReaderV1 interface {
    InspectExactToolSurfaceManifestCurrentV1(
        context.Context,
        ToolSurfaceManifestCurrentRefV1,
    ) (ToolSurfaceManifestCurrentProjectionV1, error)
}

type ToolSurfaceManifestCurrentRepositoryV1 interface {
    ToolSurfaceManifestCurrentReaderV1
    EnsureExactToolSurfaceManifestCurrentV1(
        context.Context,
        ToolSurfaceManifestCurrentEnsureRequestV1,
    ) (ToolSurfaceManifestCurrentProjectionV1, error)
}
```

公共Projection字段闭集固定为：`ContractVersion/Ref/完整Manifest/Owner/CheckedUnixNano/ExpiresUnixNano/ProjectionDigest`。不得增加Registry echo、Harness assembly echo、Model Prepared echo或弱字符串引用。

## 3. Canonical、identity与摘要

Repository不信任调用方“已Seal”：

1. deep-copy Request；
2. `Manifest.Validate()`；
3. 以Tool Surface既有canonical重算`Manifest.Digest`；
4. 以公开`ComputeExpectedInjectionDigest(Manifest.Entries)`重算并exact验证`Manifest.ExpectedInjectionDigest`；
5. 验证Entry顺序、ModelName唯一、EffectKinds排序唯一及全部Schema/Capability/Tool ref；
6. 验证`Owner == Manifest.Owner`、`ExpiresUnixNano == Manifest.ExpiresUnixNano`，以及`Ref.ID/Revision/Digest == Manifest.ID/Revision/Digest`；
7. Manifest Revision=1时`ExpectedCurrent`必须为严格零值；Revision>1时`ExpectedCurrent.Validate()`必须成功、ID相同且`Manifest.Revision == ExpectedCurrent.Revision+1`。

current stable ID直接等于`Manifest.ID`。C2禁止派生prefix、Owner散列或第二lineage；Reader入参无需Owner即可按Plan中已有ID exact定位。同一Manifest ID跨Owner、跨Manifest ContractVersion或换canonical对象必须`conflict`，不得创建第二current。

Projection digest：

```text
domain        = praxis.tool-mcp.surface-manifest-current
version       = ToolSurfaceManifestCurrentContractVersionV1
discriminator = ToolSurfaceManifestCurrentProjectionV1
body          = full Projection with only top-level ProjectionDigest cleared
```

`Ref.ID/Revision/Digest`无损复用Manifest的已有exact坐标，使Harness能直接以`Manifest.Plan.ToolSurface`坐标调用Reader；禁止latest、禁止猜Projection摘要。computed projection digest只写入`Projection.ProjectionDigest`，禁止digest回流后二次计算。`Ref.ID == Manifest.ID`、`Ref.Revision == Manifest.Revision`、`Ref.Digest == Manifest.Digest`，且Manifest digest通常不等于`ProjectionDigest`；二者不可互换。

`CheckedUnixNano`由Tool Owner clock在winner首次签发时持久化；`ExpiresUnixNano=Manifest.ExpiresUnixNano`。V1无任意默认cap、续租或caller自报TTL。同Manifest ID/revision命中不得刷新Checked/Expires，且只有winner仍是current才可幂等返回；新Manifest revision只能按`ExpectedCurrent` full Ref执行current+1 CAS。

## 4. Validators与错误闭集

```go
func (r ToolSurfaceManifestCurrentRefV1) Validate() error
func (r ToolSurfaceManifestCurrentEnsureRequestV1) Validate() error
func (p ToolSurfaceManifestCurrentProjectionV1) Validate() error
func (p ToolSurfaceManifestCurrentProjectionV1) ValidateCurrent(
    expected ToolSurfaceManifestCurrentRefV1,
    now time.Time,
) error
func SealToolSurfaceManifestCurrentV1(
    ToolSurfaceManifestCurrentProjectionV1,
) (ToolSurfaceManifestCurrentProjectionV1, error)
```

调用顺序：intrinsic → canonical/digest → 重复字段exact → expected Ref full exact → fresh current。`ValidateCurrent`要求非零fresh now、`now >= CheckedUnixNano`且`now < ExpiresUnixNano`；clock rollback或跨TTL拒绝。

closed domain errors：`invalid_argument`、`not_found`、`conflict`、`precondition_failed`、`unavailable`、`indeterminate`。只有权威`not_found`允许Ensure进入create；其余错误不得写。nil context映射`invalid_argument`。`context.Canceled`与`context.DeadlineExceeded`保留原标准cause，由Tool私有adapter边界分类，不转换成runtime/core领域类别；取消在任何外读、加锁前、post-lock及提交前均零写。typed-nil依赖在构造器确定性拒绝，不能延迟到调用时panic。

## 5. 唯一concrete Repository

同一实例原子维护：

```text
history[(Manifest.ID, Manifest.Revision)] -> immutable full Projection
current[Manifest.ID]                    -> exact current Ref
```

禁止第二仓、第二current index或Harness侧cache冒充Authority。所有Put/Get/return均deep-clone Manifest及其Entries、EffectKinds、Residuals和嵌套bytes/slices。

### 5.1 EnsureExact

```text
validate ctx + typed dependencies + deep-copied request
→ intrinsic/canonical/expected-injection validation
→ use Manifest.ID/revision directly
→ Inspect history/current
→ history hit: stable request与winner Manifest full exact
              + current Ref必须full exact等于winner.Ref
              + fresh ValidateCurrent，才return clone
              否则Conflict/Precondition，绝不回退current
→ authoritative NotFound
→ revision=1要求current NotFound；revision>1要求current full exact==ExpectedCurrent且revision=current+1
→ acquire per-stable-ID gate
→ post-lock ctx + clock check + re-read history/current
→ history hit仍执行current==winner full exact门
→ successor再次验证current==ExpectedCurrent
→ fresh Tool Owner clock
→ Seal Projection
→ atomic create history + CAS current index from ExpectedCurrent（rev1从NotFound）
→ return clone
```

同key同canonical重投仅在winner仍为current时返回winner；同Manifest ID/revision换Manifest任一字段、Manifest digest、expected injection、Owner或ContractVersion为`conflict`且零写。rev2已成为current后重投rev1必须Conflict/Precondition，绝不返回rev1或回退index。跨Owner使用相同Manifest ID也必须Conflict。不同Manifest ID不得被全局execute锁串行。

### 5.2 InspectExact

`InspectExactToolSurfaceManifestCurrentV1(ctx, expectedRef)`只读：先验证ctx/ref，直接按`expectedRef.ID`读取current index，要求current Ref与expected full exact，再读immutable winner，fresh clock执行`ValidateCurrent(expectedRef, now)`并返回clone。NotFound、Unavailable、Indeterminate不可互换；Inspect绝不调用Ensure、不得创建或推进revision。

lost Ensure reply只允许以同一canonical Request重试`EnsureExact`；Repository命中winner后执行exact验证并返回，不产生第二写。Harness M2不走此恢复口，它始终只持Ref调用Reader。

## 6. Harness M2注入合同

Harness M2构造器字段必须是：

```go
surfaceReader toolcontract.ToolSurfaceManifestCurrentReaderV1
```

禁止接Repository、concrete store、Ensure函数或Tool实现包。method-set conformance必须证明Reader只有`InspectExactToolSurfaceManifestCurrentV1`；测试double即使同时实现Repository，也要记录并断言M2全路径`EnsureCalls == 0`。M2只闭合A2、B1、Handoff、C2与Registry current；本Reader输出不替代这些门。Prepared Historical/Current只由未来M3 Gate消费，禁止进入M2构造器、DTO、canonical或Reader集合。

## 7. S1/S2与Boundary

- S1：按exact Ref调用Reader，验证完整Manifest canonical、expected injection、Owner、Ref、digest和fresh TTL。
- S2：仍按同一exact Ref重新Inspect；允许外部read取得新Checked窗口的说法不适用，本Repository winner的Checked/Expires immutable。
- actual-point：每次attempt/Open/Stream/continuation边界再InspectExact并取fresh clock；过期、rollback、Ref漂移或digest tamper均Fail Closed。
- SurfaceInvocationBinding只保存/引用该exact Ref；Binding/Ack不授执行权。

## 8. P0 slice交付门

1. 独立设计审计P0/P1/P2归零；
2. 仅在后续单独授权时实现`contract/surface_manifest_current_v1.go`与`surface/manifest_current_repository_v1.go`；
3. unit/whitebox/blackbox/fault/conformance覆盖[测试矩阵](tool-surface-manifest-current-v1-test-matrix.md)；
4. targeted ordinary×100、race×20、full ordinary/race/vet、gofmt、import-boundary、zero-network和diff-check全绿；
5. C2通过仍只关闭Repository slice，不等于Harness M2、PD-TM-04 P4、P5、system或production GO。
