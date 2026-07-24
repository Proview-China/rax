# Official SDK Result Safety V1

## 1. 结论

该切片只强化已经受Runtime V3 actual-point治理的official SDK `tools/call`回包边界，
不新增协议、不重写官方SDK Content类型，也不把Provider回包升级为DomainResult、Settlement
或ToolResult。

Tool physical executor在Provider返回后必须：

1. 接受official SDK公开`CallToolResult`；
2. 拒绝nil/typed-nil Content、仅适用于sampling的`ToolUseContent/ToolResultContent`、
   非object `structuredContent`、循环/非有限/不可编码JSON；
3. 以`min(ToolDescriptor.ResultLimitBytes, MaxMCPProtocolReceiptBytesV1)`为唯一落盘上界，
   使用有界writer生成official wire-compatible canonical JSON；
4. 禁止截断、摘要冒充完整结果或把超限内容直接灌入CLI/Context；
5. 只有完整canonical bytes可持久化时才Seal immutable `MCPProtocolReceiptV1`；否则原
   physical Entry进入`unknown`，后续只Inspect原stable key且不得重新调用Provider。

## 2. Owner与非Owner

- official SDK拥有MCP wire Content nominal与解码行为；Tool只做宿主输出限制和Receipt持久化。
- Tool Owner拥有physical Entry、Protocol Receipt、exact Inspect和Unknown水位。
- Artifact Store、Context Fragment、Application编排、Runtime Settlement及production backend
  不属于本切片。
- `MCPProtocolReceiptV1`仍是Observation/Receipt，不是正式领域结论或执行Authority。

## 3. 安全不变量

- Result Limit在Provider前已由Tool Descriptor进入canonical Command；Provider后不能扩大。
- successful canonicalization的字节数不得超过双重上界，且必须是完整合法JSON。
- `structuredContent`存在时必须编码为单一JSON object；数组、标量和null均拒绝。
- `tools/call`结果只允许official SDK声明可用于Call Result的Text/Image/Audio/ResourceLink/
  EmbeddedResource；sampling-only嵌套调用不得借此进入Tool结果。
- 错误信息不得包含Provider原始内容；Unknown reason仅保存有界固定摘要。
- CLI只输出Ref、digest、长度、状态和时间，不输出Params Inline或Canonical Response。

## 4. Artifact与背压边界

V1不伪造Artifact。超出内联上限的Provider回包当前保持Unknown并要求exact Inspect；只有未来
公共Artifact写入/读取与Owner关联合同闭合后，才可新增“完整bytes已持久Artifact、Receipt只存
exact Artifact Ref”的successor版本。未闭合前不得静默截断或把digest当作可恢复内容。

## 5. MCP Plus兼容边界

MCP+ reference package的real stdio pilot已经通过：它使用official MCP client读取标准
`tools/list`，再由`@praxis-ai/mcp-plus`外部wrapper折叠Surface。Praxis Tool/MCP不会在Go中
复制其TypeScript Exposure Planner；后续experimental adapter只消费版本化外部产物并复读
Snapshot/Surface exact refs。Assembler/Context对Capability Card、按需展开和新Surface Revision
的公共接线未闭合前，不宣称MCP+进入production Tool Surface。

## 6. 验收

- 合法Text+structured object两次canonical bytes完全一致；
- typed-nil、sampling-only、非object structured、循环/非有限JSON拒绝；
- 超限返回`precondition_failed/canonical_limit_exceeded`；
- Provider已执行但结果不可安全持久化时为Unknown、Receipt为空、二次Execute零Provider；
- targeted ordinary×100、race×20与5秒Fuzz通过；
- full ordinary/race/vet通过后才能同步module/memory，本项不解锁production。

