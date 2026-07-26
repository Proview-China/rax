# Core Pack Assembly Kit V1

## 1. 状态与目标

- Decision：accepted。
- Owner：Tool/MCP。
- Scope：owner-local SDK/Factory facade与Conformance Kit。
- Delivery：先Design PR；合并后再开Implementation PR。

CorePackAssemblyKitV1把已经存在的Core Tool Catalog、Registry、Package验证准入、
PackageAssemblyV1和ToolSurfaceManifest收成一个确定性装配入口，使Harness可以快速取得
五个官方Coding Tool的声明闭包，而不重新解释Tool领域状态。

    Core Pack config
      -> BuildCatalogV1
      -> Registry bootstrap
      -> reference preview
      -> existing verification-aware admission
      -> exact active PackageAssemblyV1
      -> exact ToolSurfaceManifest
      -> Harness test-side assembly fixture

V1不实现workspace或process Effect。无论preview还是admitted，Factory都不提供Executable
Provider；真实调用稳定返回unsupported/capability_unavailable，Provider call count为0。

## 2. 复用资产

Kit只组合现有事实和Port，不建立第二套Registry、Package或Surface语义：

| 资产 | 用途 |
|---|---|
| corepack.BuildCatalogV1 | 生成五Capability、五Tool、一个Package及Materials |
| corepack.RegisterV1 | 激活Capability/Tool并把Package留在submitted |
| sdk.PackageVerificationV1 | 走现有verification/current/admission链 |
| sdk.ResolvePackageForAssemblyV1 | 从active exact Package解析完整闭包 |
| corepack.BuildSurfaceV1 | 从exact Catalog与Registry Snapshot生成Surface |
| contract.ToolSurfaceManifest | 模型可见工具声明 |
| Harness公开Assembly合同 | 仅供消费侧fixture验证，不由生产Kit拥有 |

Kit禁止直接调用Registry私有transition绕过Package验证。RegisterV1返回submitted不是缺陷，
而是preview和admitted的安全分界。

## 3. Owner-local对象

Implementation可在tool-mcp/corepack或tool-mcp/sdk内提供以下owner-local类型；它们不是新的
跨Owner公共合同：

    CorePackAssemblyKitV1
    CorePackAssemblyRequestV1
    CorePackAssemblyResultV1
    CorePackAssemblyModeV1 = reference_preview | admitted
    CorePackAssemblyFactoryV1
    CorePackConformanceReportV1

Request只携带现有类型：CatalogConfigV1、SurfaceConfigV1的稳定输入、已有Package verification
request/current issuance/admission command、idempotency key、checked time、not-after和mode。

Result只组合：exact CatalogV1、exact Registry Snapshot Ref、Package Record、admitted模式下的
exact PackageAssemblyV1、exact ToolSurfaceManifest、五个ToolDefinitionMaterialV1、mode、
Executable=false、stable unsupported reason、canonical digest与共同最小expiry。

Result必须deep clone；同请求不得因调用方修改slice或JSON bytes而漂移。

## 4. Preview状态机

    validate request
      -> BuildCatalogV1
      -> RegisterV1
      -> confirm Package=SUBMITTED
      -> read Registry Snapshot S1
      -> build Surface bound to S1 digest
      -> read Snapshot S2
      -> require S1 == S2
      -> seal reference_preview result

Preview用于Agent Builder和Harness编译期展示。它必须同时声明reference_only=true、
admitted=false、executable=false、Package尚未active且不可绑定真实Action Provider。

Preview不得被升级为Package verification、installation、enablement或execution事实。

## 5. Admitted状态机

    build/register exact Catalog
      -> verify through existing PackageVerificationV1
      -> resolve and Inspect exact verification current
      -> admit through existing admission Port
      -> unknown/lost reply: Inspect exact Package, never blind re-admit
      -> require Package=ACTIVE and exact digest/revision
      -> read Registry Snapshot S1
      -> ResolvePackageForAssemblyV1 under S1
      -> build Surface bound to S1
      -> read Registry Snapshot S2
      -> require S1 == S2
      -> seal admitted result

Verification request、current issuance和admission command必须互相绑定同一Package
ID、Revision和Digest。Kit不得伪造签名、Trust、Credential或认证事实。

Unavailable、Indeterminate或lost reply只允许Inspect同一Package或verification坐标。无法确认
active exact winner时返回错误，不生成admitted result。

Admitted只表示Package、Tool、Capability、Schema和Surface闭包可被Harness装配；它仍不表示
workspace或process Effect可执行。

## 6. Surface与Factory边界

Surface canonical顺序固定为：

    process.exec
    workspace.inspect
    workspace.patch
    workspace.read
    workspace.search

该顺序只用于确定性序列化、Digest与缓存身份，不表示执行优先级。

Tool-owned Factory可以向Harness消费侧提供exact Package closure、五个Tool declarations、严格
Schema、Definition Materials、Surface、expiry和unsupported状态。

生产Factory不得import Harness实现、修改Harness公共合同、分配Runtime事实或拥有Harness生命周期。
Harness fixture只允许在测试文件中使用Harness现有公开合同验证Graph可编译。

## 7. 禁止边界

- 不实现file/process Effect；
- 不依赖Sandbox；
- 不import os、os/exec、io/fs、net或net/http；
- 不新增Tool、Runtime或Harness公共Owner合同；
- 不创建第二个Package Admission路径；
- 不把submitted Package当active；
- 不把preview标记为standalone或production；
- 不声明Provider、Credential、network、secret或actual-point已可用；
- 不将unsupported自动降级为bypass执行。

## 8. 失败与恢复

| 条件 | 行为 |
|---|---|
| typed-nil依赖、空clock、无效request | invalid_argument/component_missing |
| Package submitted但请求admitted | Fail Closed |
| verification/current/admission坐标漂移 | conflict/binding_drift |
| Snapshot S1/S2漂移或ABA | Fail Closed并重新创建新请求 |
| lost verification/admission reply | Inspect同一exact坐标；禁止盲重派 |
| context取消 | 原样返回，不发布Result |
| TTL在装配中到期 | precondition_failed/expired |
| Harness尝试执行 | stable unsupported；Provider call count=0 |

Kit产生的内存Catalog或Registry残留不是生产持久化事实；V1不声明durability。

## 9. Conformance Kit

Conformance Kit作为Assembly Kit测试资产一并交付，不单独形成产品：

1. canonical Catalog/Package/Surface golden；
2. preview reference-only/non-executable fixture；
3. admitted exact closure fixture；
4. Harness公开合同的test-only compile fixture；
5. unsupported execution/provider-call-zero fixture；
6. stale Snapshot、Package drift、Schema drift、ABA、TTL、cancel和typed-nil反例；
7. 64并发同请求的确定性结果。

## 10. 验收门

- 相同配置与exact Snapshot产生相同Catalog、Assembly、Surface和Result digest；
- Package到Tool、Capability、Schema、Material和Surface全链exact绑定；
- preview Package保持submitted且不可执行；
- admitted必须由现有verification-aware路径得到active Package；
- stale、drift、ABA与Unknown均Fail Closed；
- Harness fixture可编译五个声明，但执行稳定unsupported且Provider call count为0；
- production文件forbidden-import扫描通过；
- targeted ordinary count=100；
- targeted race count=20；
- full ordinary、race、vet、gofmt和diff-check全部通过。

## 11. 非目标

- 真实文件、Patch或Process Provider；
- Sandbox Port或Backend；
- MCP远程Tool混入Core Package；
- Provider-specific model lowering；
- production Registry、Credential或Secret；
- AgentDefinition字段、Harness公共Factory合同或Runtime Owner变更；
- production-ready声明。
