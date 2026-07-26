# Core Pack Preview/Inspect Product Facade V1

## 1. 状态与目标

- Decision：accepted candidate，等待 Design PR 审核。
- Owner：Tool/MCP。
- Scope：transport-neutral preview API、Go facade 与 CLI tool core-pack preview。
- Delivery：Design PR 合并后再建立独立 Implementation PR。

现有 CorePackAssemblyKit 已能生成 exact Catalog、submitted Package、Registry Snapshot 和五 Tool
Surface，但开发者仍需手工组装 Registry、SDK 与 Kit。Facade 将 reference preview 收成可发现的
产品入口；它只展示官方 Coding Tool Pack，不授 Package Admission、Surface publication 或执行权。

    strict JSON config
      -> transport-neutral Preview API
      -> CorePackAssemblyFactoryV1(reference_preview)
      -> validate exact Assembly result
      -> sealed summary + five declarations
      -> API / CLI canonical JSON

## 2. 复用与所有权

| 既有资产 | Facade 用途 |
|---|---|
| core.DecodeStrictJSON | 拒绝 unknown、duplicate、trailing JSON |
| CorePackAssemblyFactoryV1 | 唯一装配入口 |
| CorePackAssemblyResultV1 | exact、sealed、non-executable 源结果 |
| ToolSurfaceManifest | 五个模型可见声明 |
| ToolDefinitionMaterialV1 | Description 与 Input Schema exact 来源 |
| cli.RunnerV1 | 参数解析与 JSON writer |

Facade 不重建 Catalog/Surface 语义，不读取 latest，不持久化 Result，不发布 Surface Current。

## 3. CorePackPreviewConfigV1

严格配置字段固定为：

    contract_version
    owner
    artifact_digest
    signature_digest
    provenance_digest
    surface_id
    resolved_plan_digest
    profile_digest
    capability_grant_digest
    created_unix_nano
    surface_expires_unix_nano
    requested_expires_unix_nano

Registry Snapshot Digest 由 Kit 的 S1 读取生成，禁止调用方自报。Config 不包含 mode、verification、
admission、Provider、Credential、Secret、Endpoint、AgentDefinition、Harness 或 Sandbox 字段。
transport mode 固定为 reference_preview。所有时间、Owner、ID 与 Digest 显式提供；Facade 不读取
环境变量、不生成随机 ID，也不以 wall clock 改写身份。注入 Clock 只验证 currentness。

JSON Decode 必须有界并严格拒绝 unknown field、duplicate field、trailing value、错误类型、空对象、
零值与超限输入。typed API 与 JSON API 使用同一 ValidateCurrent。

## 4. CorePackPreviewResultV1

Result 是 validated Assembly Result 的 sealed 投影：

    contract_version
    assembly_digest
    package_ref + package_state
    registry_snapshot_ref
    surface_ref
    declarations[5]
    reference_only = true
    admitted = false
    executable = false
    unsupported_reason
    expires_unix_nano
    digest

每个 Declaration 固定包含 order、model_name、capability_ref、tool_ref、input_schema_ref、
description_digest、effect_kinds、risk 与 review_profile。

Result 必须逐项 exact 绑定 Assembly Result 中的 Catalog、Materials 与 Surface，且 expiry 不晚于
Assembly/Surface/config 最小窗口。替换另一组 valid Tool、Capability、Schema、Description、Effect
或 expiry 后，即使重算 Result digest 也必须被 Validate 拒绝。所有 slice 与 JSON bytes deep clone。

## 5. API 状态机

    validate context and dependencies
      -> strict decode or typed Config ValidateCurrent
      -> map to existing preview Assembly Request
      -> Factory.BuildV1
      -> require preview/submitted/reference_only/non-executable
      -> validate exact five-tool closure
      -> project and seal Preview Result
      -> fresh TTL/clock check
      -> return deep clone

相同 Config、同一 Registry 状态与同一显式 Clock 必须产生相同 Assembly 和 Preview digest。Registry
在装配中变化时复用 Kit S1/S2 Fail Closed，不自动重试或读取 latest。

## 6. CLI 合同

首批命令唯一为：

    tool core-pack preview --config-json <strict-json>

内联 JSON 复用当前 RunnerV1(args, writer)，不引入文件、stdin、环境变量或 Secret 读取。成功时
stdout 只写一个 canonical CorePackPreviewResultV1 JSON 和换行；失败时 stdout 为零字节。

以下命令必须稳定 unsupported：

    tool core-pack admit
    tool core-pack enable
    tool core-pack execute
    tool core-pack publish

## 7. 安全与失败边界

全链必须保持 reference_only=true、admitted=false、executable=false，并证明 Provider、Sandbox、
Surface Current write 调用数都为 0。unsupported_reason 原样继承 Assembly Kit，不提供 shell、文件
API、测试 Provider 或 Hook 降级路径。

| 条件 | 行为 |
|---|---|
| malformed/unknown/duplicate/trailing JSON | invalid_argument；零输出 |
| typed-nil、nil context | component_missing / invalid_reference |
| canceled context | 原样返回；零 Result |
| TTL/clock 回退 | precondition_failed |
| S1/S2 或 exact closure 漂移 | conflict / binding_drift |
| admitted/publish/execute 命令 | unsupported / capability_unavailable |

Preview 无外部 Effect，因此没有 UnknownOutcome 重试协议。Facade 不把 Unavailable/Conflict 改写成
NotFound，也不重新派发 Kit 调用。

## 8. 包边界与验收

Implementation 预计只涉及 Tool-owned api、cli 及对应测试：

    ExecutionRuntime/tool-mcp/api/core_pack_preview_v1.go
    ExecutionRuntime/tool-mcp/cli/core_pack_preview_v1.go
    ExecutionRuntime/tool-mcp/tests/conformance/core_pack_preview_v1_test.go

生产文件禁止新增 Harness、Sandbox、AgentDefinition、Assembler、Context、Memory、os、os/exec、
io/fs、net、net/http 或 Runtime private 实现依赖。

验收必须覆盖：

1. strict JSON valid/unknown/duplicate/trailing/oversized；
2. deterministic digest 与五声明 golden；
3. exact closure forge/drift；
4. TTL、clock、S1/S2、cancel、typed-nil；
5. deep clone 与 64 并发同结果；
6. CLI 成功单 JSON、失败零 stdout、危险子命令 unsupported；
7. Provider/Sandbox/Surface Current write count = 0；
8. targeted ordinary x100、race x20、full ordinary/race/vet、forbidden-import、diff-check。

## 9. 非目标

- admitted transport、Verify/Admission/Enable；
- Surface Current publication；
- Core Tool Provider、Sandbox、MCP Surface composition；
- SQLite Registry/Result Store；
- AgentDefinition Preset、Harness/Model注入、daemon/HTTP或production root。

后续顺序保持：Preview Facade -> Local State Pack Design -> 经 A 级裁决的 Core+MCP Composer。
