# Core Tool Pack V1：最小本地编码工具包

状态：`design_pr_candidate`。本文件只冻结首个官方工具包的产品语义与实施合同；
在本Design PR合并并获得Implementation授权前，不创建Provider、生产配置或可执行入口。

## 1. 目标与边界

Core Tool Pack V1把本地编码任务中最常用的五个能力装配为正式
`CapabilityDescriptor -> ToolDescriptor -> ToolPackageManifest -> ToolSurfaceManifest`
对象，并复用既有Single Call Action、Runtime治理、Review与Sandbox门禁。

V1只包含：

1. `workspace.read`
2. `workspace.search`
3. `workspace.inspect`
4. `workspace.patch`
5. `process.exec`

V1明确不包含browser/web、Git push/GitHub、database、邮件、部署、删除型工具、
外发网络、任意Secret、远程服务或远程MCP Tool。MCP发现出的同名能力不得覆盖、
冒充或自动升级为Core Tool。

## 2. 稳定身份与Registry对象

模型可见名称与Registry精确身份是两个字段，禁止互换：

| 模型可见名 | Capability ID | Local ToolDescriptor ID | Risk | 默认Review |
|---|---|---|---|---|
| `workspace.read` | `praxis.core-tool/workspace-read` | `praxis.core-tool/workspace-read-local-v1` | `low` | `bypass` |
| `workspace.search` | `praxis.core-tool/workspace-search` | `praxis.core-tool/workspace-search-local-v1` | `low` | `bypass` |
| `workspace.inspect` | `praxis.core-tool/workspace-inspect` | `praxis.core-tool/workspace-inspect-local-v1` | `low` | `bypass` |
| `workspace.patch` | `praxis.core-tool/workspace-patch` | `praxis.core-tool/workspace-patch-local-v1` | `moderate`，越scope为`high` | `auto`，高风险升级`human` |
| `process.exec` | `praxis.core-tool/process-exec` | `praxis.core-tool/process-exec-local-v1` | `moderate`，越policy为`high` | `auto`，network/secret/host escape为`human_or_deny` |

共同冻结：

- Package ID：`praxis.core-tool/local-coding-pack-v1`；
- SemVer：首版`1.0.0`，Registry Revision从`1`开始；
- Mechanism：`local`；
- EffectKind：复用`praxis.tool/execute`，不得为五个工具另造Runtime Effect链；
- Tool Owner：`tool-mcp`；实际文件/进程Effect由Sandbox Provider执行；
- `bypass`不是跳过治理，而是必须取得Review Owner的
  `operation_not_required` current projection；
- `auto|human|human_or_deny`是Tool Owner声明的Review默认类别，最终Verdict与
  exact Policy/Profile Ref仍由Review Owner产生；
- Package进入Registry不等于进入Agent Plan；进入Surface不等于获得执行授权。

## 3. 共同坐标与安全默认

每个输入都必须绑定以下中立坐标，具体公共nominal由Implementation前联合编译检查确认，
Tool不得复制Sandbox、Review或Runtime共享类型：

| 字段 | 约束 |
|---|---|
| `workspace_root` | exact ID/Revision/Digest/currentness；只能指向本次Operation获准的workspace root |
| `relative_path` | UTF-8，1..4096 bytes；必须clean、相对、无NUL、无`..`逃逸 |
| `requested_not_after` | 正数；最终TTL取Authority、Scope、Fence、Sandbox Lease、workspace current与caller deadline最小值 |
| `operation_scope_digest` | 与Action Candidate、Review Target、Runtime Intent及Sandbox执行点exact一致 |

全包默认：

- workspace外：`deny`；
- network：`deny`；
- secret：`deny`；
- symlink：允许读取symlink自身metadata；禁止follow后逃出workspace root；
- host escape、device file、socket、procfs/sysfs或未获准mount：`deny`；
- Provider实际入口必须重新Inspect Authority、Scope、TTL、Fence、Review和Sandbox
  current；任一Unavailable/NotFound/漂移/过期均Fail Closed；
- UnknownOutcome只Inspect原Attempt，不自动重派；
- inline结果超过当前工具上限时必须转为exact Artifact Ref；不允许截断后伪称完整。

## 4. 精确输入输出Schema

Schema使用严格JSON：未知字段、重复key、非规范数字、超限数组和尾随内容全部拒绝。
所有byte计数指UTF-8/原始文件字节，不以rune数量替代。

### 4.1 `workspace.read`

输入`praxis.core-tool.schema/workspace-read-input@1.0.0`：

```json
{
  "workspace_root": {"id": "...", "revision": 1, "digest": "sha256:..."},
  "relative_path": "src/main.go",
  "start_byte": 0,
  "max_bytes": 65536,
  "requested_not_after_unix_nano": 1
}
```

- `start_byte`默认`0`；
- `max_bytes`默认`65536`，范围`1..1048576`；
- 读取前后S1/S2复读同一file ID/Revision/Digest/currentness；变化返回Conflict，不拼接两版内容；
- 目录、逃逸symlink、device/socket或超出授权root拒绝。

输出`praxis.core-tool.schema/workspace-read-output@1.0.0`：

```json
{
  "file": {"id": "...", "revision": 1, "digest": "sha256:..."},
  "start_byte": 0,
  "bytes_returned": 0,
  "total_bytes": 0,
  "complete": true,
  "content": "",
  "artifact_ref": null
}
```

`content`与`artifact_ref`二选一；`complete=false`必须给出真实`total_bytes`和range，
不能被Context解释为完整文件。

### 4.2 `workspace.search`

输入`praxis.core-tool.schema/workspace-search-input@1.0.0`：

```json
{
  "workspace_root": {"id": "...", "revision": 1, "digest": "sha256:..."},
  "query": "ToolDescriptor",
  "path_prefix": "",
  "mode": "literal",
  "max_results": 50,
  "max_result_bytes": 262144,
  "requested_not_after_unix_nano": 1
}
```

- `query`为1..4096 bytes；V1仅支持`literal|regexp-re2`；
- `max_results`默认`50`、最大`200`；
- `max_result_bytes`默认`262144`、最大`1048576`；
- 路径按canonical relative path排序，同路径按byte offset排序；
- 不搜索workspace外、symlink逃逸目标、binary正文或未授权hidden scope。

输出`praxis.core-tool.schema/workspace-search-output@1.0.0`：

```json
{
  "workspace_revision": 1,
  "workspace_digest": "sha256:...",
  "matches": [
    {"path": "src/main.go", "file_revision": 1, "file_digest": "sha256:...", "start_byte": 0, "end_byte": 14, "preview": ""}
  ],
  "complete": true,
  "artifact_ref": null
}
```

超过上限时`complete=false`并返回Artifact Ref；不得只丢弃尾部而仍标完整。

### 4.3 `workspace.inspect`

该工具只检查文件/目录metadata、bounded range可用性与currentness，不返回Tool自描述，
也不读取大正文。

输入`praxis.core-tool.schema/workspace-inspect-input@1.0.0`：

```json
{
  "workspace_root": {"id": "...", "revision": 1, "digest": "sha256:..."},
  "relative_path": ".",
  "range": {"start_byte": 0, "max_bytes": 65536},
  "max_entries": 200,
  "requested_not_after_unix_nano": 1
}
```

- `range`只验证给定range是否落在当前文件长度内，不返回正文；
- `max_entries`默认`200`、最大`1000`；
- 目录结果只列直属成员；V1不递归；
- symlink返回`kind=symlink`与link metadata，不follow。

输出`praxis.core-tool.schema/workspace-inspect-output@1.0.0`：

```json
{
  "object": {"path": ".", "kind": "directory", "revision": 1, "digest": "sha256:...", "size_bytes": 0, "mode": 0, "modified_unix_nano": 1},
  "range_valid": true,
  "entries": [],
  "complete": true
}
```

### 4.4 `workspace.patch`

输入`praxis.core-tool.schema/workspace-patch-input@1.0.0`只接受结构化hunk：

```json
{
  "workspace_root": {"id": "...", "revision": 1, "digest": "sha256:..."},
  "changes": [
    {
      "relative_path": "src/main.go",
      "base_revision": 1,
      "base_digest": "sha256:...",
      "hunks": [
        {"old_start": 1, "old_lines": 1, "new_start": 1, "new_lines": 1, "lines": [{"op": "context", "text": "package main"}]}
      ]
    }
  ],
  "requested_not_after_unix_nano": 1
}
```

- `changes`为1..64，路径唯一且canonical排序；
- 总patch canonical bytes最大`1048576`，单文件hunk最大`1024`；
- `op`闭集为`context|delete|insert`；不接受command、shell text、任意script或binary patch；
- 每个文件必须在写前复读exact `base_revision+base_digest`；
- 所有文件先完整validate/stage，再由Sandbox Workspace Owner执行一次原子CAS提交；
- 任一文件漂移、越scope、逃逸symlink、Review/Fence过期时全批次零写；
- V1不支持删除文件、rename、chmod、symlink创建或binary写入。

输出`praxis.core-tool.schema/workspace-patch-output@1.0.0`：

```json
{
  "change_set_ref": {"id": "...", "revision": 1, "digest": "sha256:..."},
  "base_workspace": {"id": "...", "revision": 1, "digest": "sha256:..."},
  "result_workspace": {"id": "...", "revision": 2, "digest": "sha256:..."},
  "files": [{"path": "src/main.go", "revision": 2, "digest": "sha256:..."}]
}
```

### 4.5 `process.exec`

输入`praxis.core-tool.schema/process-exec-input@1.0.0`：

```json
{
  "workspace_root": {"id": "...", "revision": 1, "digest": "sha256:..."},
  "argv": ["go", "test", "./..."],
  "cwd": ".",
  "env": {"GOFLAGS": "-mod=readonly"},
  "timeout_millis": 30000,
  "max_stdout_bytes": 262144,
  "max_stderr_bytes": 262144,
  "requested_not_after_unix_nano": 1
}
```

- `argv`为1..128，每项1..8192 bytes，总和最大`65536`；
- 默认无隐式shell，禁止`sh -c`式自动包装、字符串插值、重定向解析和管道解析；
- `argv[0]`必须由Sandbox当前Executable Allowlist解析，并绑定实际artifact digest；
- `cwd`是workspace-root scoped canonical相对路径；
- `env`最多64项，只允许Sandbox current policy中的key；不继承未声明宿主环境；
- `timeout_millis`默认`30000`，范围`1..300000`；
- stdout/stderr默认各`262144`，单项最大`1048576`；
- network、secret、host namespace/device/mount escape默认拒绝；
- shell语义只能由未来显式profile/adapter开启，并仍走Sandbox/Review治理；
- 若保留历史名`shell.run`，它只能是装配期Alias，必须解析到同一
  `praxis.core-tool/process-exec-local-v1` exact Descriptor，禁止第二执行语义。

输出`praxis.core-tool.schema/process-exec-output@1.0.0`：

```json
{
  "attempt_ref": {"id": "...", "revision": 1, "digest": "sha256:..."},
  "exit_code": 0,
  "stdout": "",
  "stderr": "",
  "stdout_artifact_ref": null,
  "stderr_artifact_ref": null,
  "timed_out": false
}
```

每个stream的inline与Artifact Ref二选一。进程启动后丢失回包进入UnknownOutcome，
只能Inspect原Attempt。

## 5. Review、Sandbox与执行顺序

统一顺序：

```text
Registry exact objects
  -> Tool Surface exact manifest
  -> Model Tool Call Observation
  -> ActionCandidateV3 / BindingV2
  -> Runtime Intent / Admission / Permit / Begin
  -> Review current projection
  -> Sandbox current + actual-point enforcement
  -> Sandbox Provider Observation / Receipt
  -> Tool Owner DomainResult
  -> Runtime Settlement V4 current closure
  -> Tool ApplySettlement / settled ToolResultV2
  -> neutral Context projection
```

读工具的`bypass`也必须有`operation_not_required` current projection，并在实际点复读。
`workspace.patch`和`process.exec`默认Auto Review；以下任一条件必须升级Human或Deny：

- scope扩大、目标digest漂移或无法确认base；
- network、secret、host escape、额外mount/device；
- executable不在allowlist或artifact digest不可证；
- patch触及高风险Policy路径；
- Review、Authority、Fence、Sandbox或workspace current不可读/过期。

## 6. Settled Tool Result中立投影

Tool Owner新增消费者中立的`SettledToolResultProjectionV1`，只做settled结果的只读交接，
不定义Context Fragment、Context Frame或Runtime Outcome语义：

| 字段 | 语义 |
|---|---|
| `contract_version` | `praxis.tool-mcp.settled-tool-result-projection/v1` |
| `tool_result` | exact settled `ToolResultV2` Ref |
| `tool` | exact Core ToolDescriptor Ref |
| `operation_inspection` | current `OperationInspectionSettlementRefV4` |
| `payload_schema`、`payload_digest`、`payload_revision` | 与ToolResult exact一致 |
| `inline_payload`或`artifact_ref` | 二选一；受当前limit policy约束 |
| `complete` | false时Context不得当作完整结果 |
| `classification_digest` | 数据分类投影，不携Secret原文 |
| `checked_unix_nano`、`expires_unix_nano`、`projection_digest` | current窗口与canonical摘要 |

只有Tool Owner的settled ToolResult与Runtime V4 current closure同时有效时才可签发。
该Projection只公开exact settled result、current closure、inline/artifact二选一、
classification、completeness和TTL。Context只是潜在消费者之一；任何消费者均须独立Inspect。
Context Owner若消费它，仍必须经既有Refresh/S2/Generation CAS形成自己的事实；Tool不得直接
写Context Fragment、Context Frame或Harness Continuation。

## 7. Registry、Surface与MCP边界

- 五个Capability、五个Local ToolDescriptor与一个Package按依赖顺序进入Registry；
- Tool Surface使用`contract.SealSurface`的canonical顺序：
  `process.exec, workspace.inspect, workspace.patch, workspace.read, workspace.search`；
- 该顺序只用于确定性序列化、Digest与缓存身份，不表示执行优先级或调度顺序；
- Surface必须引用上述exact Descriptor、Schema digest与Description digest；
- Core Pack V1不创建`MCPToolMappingManifestV1`，不把远程MCP Tool纳入Package；
- MCP发现到同名Tool时必须使用其MCP来源身份，不能shadow Core Tool；
- 未来若把Core Tool对外暴露为MCP Server，需独立设计Server、Credential、Network、
  Disclosure和Lifecycle，不在V1内。

## 8. 错误闭集

| Category | 使用场景 | 是否可自动重试 |
|---|---|---|
| `invalid_argument` | Schema、path、range、argv、patch格式非法 | 否 |
| `forbidden` | workspace/network/secret/symlink/host escape或Policy拒绝 | 否 |
| `not_found` | exact workspace/file/executable对象权威不存在 | 仅新调用可重新解析；同Attempt不盲重派 |
| `conflict` | revision/digest、base、Surface、Descriptor或幂等内容漂移 | 否，重新生成Candidate |
| `precondition_failed` | TTL/Fence/Review/Sandbox/currentness过期 | 否，重新走治理 |
| `capability_unavailable` | Provider/Profile/Adapter未装配 | 否 |
| `rate_limited` | 并发或预算上限 | 由上层Policy决定，不能跨过原TTL |
| `unavailable` | current Reader/State Plane暂不可读 | 只Inspect，不等于NotFound |
| `indeterminate` | Effect可能已发生但Receipt不确定 | 只Inspect原Attempt |
| `internal` | 不变量破坏 | 否；零扩权并保留审计 |

context cancellation保留`context.Canceled`或`DeadlineExceeded`，不伪装成新的core category。

## 9. 实施Owner与非Owner

| Owner | 本纵切职责 | 明确禁止 |
|---|---|---|
| Tool/MCP | Core Descriptor/Schema/Package/Surface、Result规范化、DomainResult/Apply/Inspect、消费者中立Settled Result Projection | 不执行Sandbox Effect，不写Review/Context/Runtime事实，不直接调用OS文件/进程API |
| Runtime | Intent、Admission、Permit、Begin、Fence、Settlement V4 | 不解释Tool payload，不选择Tool领域Disposition |
| Review | Bypass/Auto/Human Policy与current Verdict | 不Dispatch Provider |
| Sandbox | workspace/process current、Scope强制、原子patch、进程actual-point | 不注册或改写Tool语义 |
| Context | 独立Inspect中立投影并形成Frame | 不把Provider回包直接升级为Context事实 |
| Harness/Application | 组装、Candidate与跨Owner顺序 | 不import Tool实现，不绕过Gateway |

当前Design PR不新增跨Owner公共Go合同。Implementation开始前若现有Sandbox workspace/process
读写Port无法无损承载root current、base CAS、Executable allowlist或actual-point TTL，
必须提交结构化Port Delta并保持对应Capability/Adapter unsupported，禁止Tool本地复制共享类型。
Tool生产文件只允许通过Sandbox公开Port请求Effect；不得直接调用`os/exec`、OS文件写口、
进程启动API或在Tool包内实现Sandbox Effect。

## 10. 硬反例

1. absolute path、`..`、symlink逃逸、TOCTOU换root后仍读写；
2. read/search/inspect没有Review `operation_not_required` current projection；
3. patch缺base revision/digest、部分文件先写、失败后留下半批；
4. patch夹带shell、binary、delete、rename或chmod；
5. `process.exec`把字符串送入shell、继承宿主env或解析重定向/管道；
6. network/secret/host escape在Auto Review下执行；
7. stdout/stderr或文件正文超限仍直接塞Context；
8. Provider回包直接形成ToolResult、Context Frame或Runtime Outcome；
9. lost reply/Unknown创建新Attempt；
10. MCP同名Tool覆盖Core exact Descriptor或进入Core Package；
11. `shell.run`形成独立Provider/Descriptor而非同exact Tool Alias；
12. Result projection缺ToolResult/Settlement closure或过期仍被Context消费。

## 11. 本Design PR的完成条件

- 五个模型名、Capability ID、ToolDescriptor ID、Package ID及顺序唯一；
- Schema、limits、Review默认、Sandbox要求、错误闭集与Result投影完整；
- 明确无网络、无Secret、无远程MCP、无生产Backend/root；
- Design/plan局部链接、draw.io XML、Markdown相对链接与`git diff --check`通过；
- PR合并仅表示设计确认，不表示Implementation或production GO。
