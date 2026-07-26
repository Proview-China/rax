# Core Tool Pack V1 实施计划

状态：`design_pr_candidate`。本计划只对应
[Core Tool Pack V1设计](../../design/tool-engine/core-tool-pack-v1.md)；Design PR合并后，
仍需总控下发Implementation授权才可写Go。

## 1. 交付边界

实现后预期交付一个可嵌入的Go官方本地编码工具包：

- 五个正式Capability/Tool Descriptor与严格Schema；
- 一个Package Manifest及Registry Admission；
- 确定性Tool Surface；
- Tool Owner消费者中立`SettledToolResultProjectionV1`；
- Sandbox Provider窄adapter；
- SDK/API只通过既有Governed Action链调用；
- 单元、白盒、黑盒、故障、Conformance、Race与Vet证据。

不交付production root、持久数据库、browser/web、网络、Secret、远程MCP Server、
GitHub/database/邮件/部署/删除型工具或系统级SLA。

## 2. 文件级落点

仅允许修改`ExecutionRuntime/tool-mcp/**`与Tool/MCP自有资产：

| 候选文件 | 产物 |
|---|---|
| `contract/core_tool_pack_v1.go` | 五工具ID、Package ID、Review默认、限制常量和共享坐标 |
| `contract/core_tool_schemas_v1.go` | 五组严格Input/Output DTO、Validate/Seal/Clone |
| `contract/settled_tool_result_projection_v1.go` | Tool-owned消费者中立Settled Result投影、current/digest验证 |
| `corepack/catalog_v1.go` | Descriptor/Schema/Package确定性构造 |
| `corepack/surface_v1.go` | 固定顺序Surface构造与exact material |
| `corepack/registry_v1.go` | 按Capability→Tool→Package的Admission编排 |
| `corepack/workspace_sandbox_adapter_v1.go` | 只调用Sandbox公开Port的read/search/inspect/patch窄adapter；不直接读写OS文件 |
| `corepack/process_sandbox_adapter_v1.go` | 只调用Sandbox公开Port的argv-oriented窄adapter；不启动OS进程、无shell fallback |
| `sdk/core_tool_pack_v1.go` | Governed Action/Inspect façade |
| `api/core_tool_pack_v1.go` | transport-neutral调用与只读Inspect |
| `internal/testkit/core_tool_pack_v1.go` | 仅测试fixture与计数器 |
| `tests/{blackbox,fault,conformance}/core_tool_pack_v1_test.go` | 外部行为、故障与边界门 |

若Sandbox公共Port不足，只在
`.properties.rax/design/tool-engine/port-delta.md`登记结构化Delta；不得在Tool包复制
Sandbox/Review/Runtime类型或导入其实现。

## 3. 阶段

### P0：Design PR

- [x] 冻结五个模型名、Capability/Tool/Package ID；
- [x] 冻结严格Schema、上限、Review默认与Sandbox要求；
- [x] 冻结错误闭集、Result→Context中立投影与MCP非归属；
- [x] 冻结Owner、依赖、文件落点与测试矩阵；
- [ ] 总控审查并合并Design PR。

### P1：合同与Catalog

- [ ] 落ID、DTO、Schema、canonical digest、clone与错误；
- [ ] 构造五Capability、五Local ToolDescriptor、一个Package；
- [ ] 同ID同内容幂等，同ID换内容Conflict；
- [ ] 不包含MCP Mapping、network、Secret或shell第二语义。

### P2：Registry与Surface

- [ ] Capability→Tool→Package按顺序Admission；
- [ ] 固定Surface顺序与ModelName唯一性；
- [ ] exact Schema/Description/Descriptor digest；
- [ ] `shell.run`若保留，只作指向`process.exec` exact Descriptor的装配期Alias；
- [ ] MCP同名能力不可shadow Core对象。

### P3：Workspace Sandbox Adapter

- [ ] 只通过Sandbox公开Port复读root current、canonical path与symlink-safe结果；
- [ ] read S1/S2 exact、range与Artifact化；
- [ ] search bounded、稳定排序与binary拒绝；
- [ ] inspect metadata/range/currentness，不读大正文；
- [ ] patch完整validate/stage后单次原子CAS；
- [ ] workspace外、TOCTOU、base drift、partial commit全部Fail Closed。
- [ ] Sandbox Port不足时Capability保持unsupported并提交Port Delta；Tool不直接调用OS文件API。

### P4：Process Sandbox Adapter

- [ ] 只接受argv数组；
- [ ] exact executable allowlist与artifact digest；
- [ ] cwd root scope、env allowlist、timeout/output cap；
- [ ] network/secret/host escape拒绝；
- [ ] 实际入口fresh复读Review/Sandbox/Fence/TTL；
- [ ] lost reply只Inspect原Attempt。
- [ ] Sandbox Port不足时Capability保持unsupported并提交Port Delta；Tool不直接调用
  `os/exec`或其他进程启动API。

### P5：Result与开发者入口

- [ ] Provider Observation→Tool DomainResult→Runtime Settlement→Tool Apply；
- [ ] 仅settled ToolResult+Runtime V4 current closure签发消费者中立
  `SettledToolResultProjectionV1`；
- [ ] 大结果只交Artifact Ref；
- [ ] SDK/API不提供Provider直连后门；
- [ ] Tool不写Context Frame或Harness Continuation。

### P6：发布候选

- [ ] module/memory同步；
- [ ] independent review P0/P1/P2归零；
- [ ] release mode保持`standalone`或更低；
- [ ] production root/backend、持久化、系统SLA继续NO-GO。

## 4. 测试矩阵

| 层级 | 必测内容 | 通过标准 |
|---|---|---|
| 单元 | 五组DTO、严格JSON、canonical、digest、limits、deep clone | 每个非法字段Fail Closed，无panic |
| Catalog | 5 Capability/5 Tool/1 Package ID、Schema、Review/Sandbox | golden exact；换任一字段digest变化 |
| Registry白盒 | submit/admit/current/revoke、CAS、Alias | 同内容幂等；同ID换内容Conflict |
| Surface白盒 | 固定顺序、唯一ModelName、MCP同名冲突 | Core exact对象不被shadow |
| workspace.read | range边界、S1/S2漂移、Artifact化 | 不跨版拼接，不越root |
| workspace.search | literal/RE2、排序、数量/字节cap | `complete`与Artifact真实 |
| workspace.inspect | file/dir/symlink metadata、range valid | 不返回大正文，不follow逃逸symlink |
| workspace.patch | base CAS、多文件原子、结构化hunk | 失败全批零写，不接受shell/delete/rename |
| process.exec | argv/cwd/env/timeout/output | 无隐式shell、无宿主env/network/secret |
| Review | bypass/auto/human升级与TTL | 无current projection时Provider=0 |
| Sandbox | root、symlink、executable、Fence实际点 | 任一漂移Provider=0 |
| Result | Observation/DomainResult/Settlement/Apply/Settled Result投影 | Provider回包不能越级；投影不定义Context语义 |
| Unknown | provider后各lost-reply点 | 只Inspect，原Attempt最多一次Effect |
| 黑盒 | SDK调用五工具的成功/拒绝/Unknown | 不暴露内部handle或直连Provider |
| 故障 | Reader unavailable、clock rollback、TTL crossing、CAS丢回包 | 零扩权、可审计恢复 |
| 并发 | 同key 64并发、不同key并行、同文件patch竞争 | 同key单Effect；写冲突单赢家 |
| Conformance | import、method-set、无network/secret/shell backdoor | 禁止跨Owner实现导入和第二执行语义 |
| Effect Owner/import | 扫描Tool production文件的`os/exec`、OS文件mutation、进程启动与Sandbox实现导入 | Tool只能调用Sandbox公开Port；命中即失败，对应Capability保持unsupported |
| Race/Vet | 全模块 | `go test -race ./...`、`go vet ./...`通过 |

## 5. 定向命令

Implementation阶段实际执行并记录结果：

```bash
cd ExecutionRuntime/tool-mcp
go test ./corepack ./contract ./registry ./surface -count=100
go test -race ./corepack ./tests/blackbox ./tests/fault ./tests/conformance -count=20
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
gofmt -w <本轮Go文件>
git diff --check
```

没有benchmark证据前不引入Rust。只有Go profile证明canonical、search或patch热点达不到
已确认目标后，才提交独立benchmark与Rust/FFI或进程隔离设计；V1默认Go。

## 6. 实现前Gate

Implementation开始前必须同时满足：

1. 本Design PR已由总控审查并合并；
2. 总控明确下发Implementation授权；
3. live Sandbox公共Port能无损表达workspace root current、symlink-safe解析、
   patch原子CAS、Executable allowlist和actual-point TTL；否则对应Capability/Adapter保持
   unsupported并提交Port Delta，Tool不得直接实现文件/进程Effect；
4. Review现有公共合同能表达`operation_not_required|auto|human` current决策；
5. Context只消费Tool中立投影，不要求Tool导入Context实现；
6. production root、网络、Secret和远程MCP没有被本计划暗中启用。

## 7. 验收结论

本计划完成后，开发者可以通过同一Tool Registry与Surface声明并调用五个本地编码工具，
但所有实际Effect仍受Runtime、Review和Sandbox共同门禁。它不会产出通用shell、网络工具、
生产后端或“开箱即production ready”的声明。
