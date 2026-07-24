# MCP 2025-06-18降级Conformance V1

## 1. 结论

Praxis Tool/MCP已经用`github.com/modelcontextprotocol/go-sdk v1.6.1`的公开协议类型，
验证`2025-06-18`的`initialize`、`tools/list`和`tools/call`可无损穿过本模块严格
JSON-RPC codec。该结果是wire/type兼容证据，不改变正式受治理Connect、Discovery与Call链
继续锁定`2025-11-25`的事实，也不构成长期兼容期限、production Transport或SLA承诺。

## 2. 组装边界

- 输入/输出协议对象直接使用official Go SDK public
  `InitializeParams/InitializeResult`、`ListToolsParams/ListToolsResult`、
  `CallToolParams/CallToolResult`，Tool领域合同不复制这些nominal；
- Praxis只负责严格JSON-RPC envelope、ID、大小、canonical JSON、Schema与版本重叠校验；
- `NegotiateProtocol("2025-06-18", ["2025-11-25", "2025-06-18"])`必须精确选择
  `2025-06-18`；只有`2025-11-25`时必须Fail Closed，不得伪造重叠；
- official SDK v1.6.1虽然内部支持`2025-06-18`，但公共`Client.Connect`没有可设置旧版本的
  exported option。因此本版本不宣称“真实public ClientSession降级已跑通”，也不通过反射、
  `unsafe`、vendor internal或复制SDK代码绕过该边界。

## 3. 已验证对象

1. official `InitializeParams` JSON可被Praxis严格解码并协商旧版本；
2. Praxis `InitializeResult` JSON可被official `InitializeResult`完整消费；
3. official `tools/list` cursor、Tool name/description/InputSchema可往返，Schema继续经过
   Praxis深度、节点和canonical限制；
4. official `tools/call` name/arguments与text result可往返；
5. formal `MCPStableProtocolVersion`仍为`2025-11-25`，正式Connect/Discovery/Call actual-point
   没有被该测试放宽。

## 4. 未证明与后续门

- 未证明official public Client/Server真实Session在`2025-06-18`下完成Connect、分页和Call；
- 未承诺`2025-06-18`的Tasks、扩展字段、认证或Streamable HTTP全部行为；
- 若上游SDK未来公开旧版本Client option，可增加真实in-memory/stdio/HTTP session Conformance，
  但仍须由Server Descriptor exact范围与Runtime actual-point治理决定是否启用；
- production兼容窗口、弃用期限和Server准入仍由管理线冻结。

## 5. 验收

- `TestConformanceMCP20250618OfficialTypesInitialize`；
- `TestConformanceMCP20250618OfficialTypesListAndCall`；
- `TestConformanceMCP20250618DoesNotWidenStableChain`；
- targeted ordinary×100、race×20、full ordinary/race、vet、gofmt、import boundary与
  `git diff --check`。

