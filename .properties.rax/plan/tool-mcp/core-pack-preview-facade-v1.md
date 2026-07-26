# Core Pack Preview/Inspect Product Facade V1 Plan

## 1. 目标

把已合并的 CorePackAssemblyKit reference preview 包装成 Tool-owned API 与 CLI，使开发者无需
直接组装 Registry/SDK/Kit 即可查看五个官方 Coding Tool 声明。

## 2. 文件边界

Design PR 只修改本 Design 与 Plan。Design 合并后的 Implementation PR 预计只修改 Tool-owned：

    ExecutionRuntime/tool-mcp/api/core_pack_preview_v1.go
    ExecutionRuntime/tool-mcp/api/core_pack_preview_v1_test.go
    ExecutionRuntime/tool-mcp/cli/core_pack_preview_v1.go
    ExecutionRuntime/tool-mcp/cli/core_pack_preview_v1_test.go
    ExecutionRuntime/tool-mcp/tests/conformance/core_pack_preview_v1_test.go

不得修改 AgentDefinition、Harness、Runtime、Sandbox、Context、Memory 或跨 Owner 公共合同。

## 3. 阶段

### P0：Design PR

- [x] reference-preview-only；
- [x] strict Config 与 sealed Result；
- [x] CLI tool core-pack preview --config-json；
- [x] non-executable/provider-call-zero；
- [x] 错误闭集与测试矩阵；
- [ ] 主控审核并合并 Design PR。

### P1：API facade

- [ ] strict decode 与 ValidateCurrent；
- [ ] 复用 Assembly Factory；
- [ ] exact 投影五 Declaration；
- [ ] seal/Validate/deep clone；
- [ ] deterministic、forge、TTL、cancel、typed-nil、64 并发测试。

### P2：CLI

- [ ] 接入 tool core-pack preview --config-json；
- [ ] 成功单 JSON，失败零 stdout；
- [ ] admit/enable/execute/publish unsupported；
- [ ] 不读取文件、stdin、环境变量或 Secret。

### P3：软件门

- [ ] Provider/Sandbox/Surface Current write count = 0；
- [ ] targeted ordinary x100、race x20；
- [ ] full ordinary/race/vet；
- [ ] forbidden-import、gofmt、diff-check；
- [ ] Draft Implementation PR，不合并。

## 4. NO-GO

- Design PR 未合并前不写 Go；
- 不开放 admitted、enable、execute、publish；
- 不发布 Surface Current，不实现 Provider，不依赖 Sandbox；
- 不新增 AgentDefinition 字段或默认 Preset；
- Local State Pack 与 Core+MCP Composer 不混入本 PR。
