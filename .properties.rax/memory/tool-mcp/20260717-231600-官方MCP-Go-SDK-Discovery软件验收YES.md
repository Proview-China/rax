# 官方MCP Go SDK Discovery软件验收YES

## 事件

- Tool Owner直接组装`github.com/modelcontextprotocol/go-sdk v1.6.1`，新增注入式initialized ClientSession Discovery Adapter；
- 新增`MCPCapabilitySnapshotV2`，将Tools、Resources、Prompts保持为三个独立typed Observation nominal；
- Provider列表顺序不进入语义：三类对象先规范排序，再计算SourceDigest并Seal Snapshot；
- Adapter只调用官方SDK的ListTools/ListResources/ListPrompts，不创建stdio/HTTP连接、不导出raw Transport/Connect入口、不调用Provider Tool；
- 分页、cursor环、对象/页数上限、duplicate、nil、协议/Session漂移、TTL crossing、clock rollback、canceled context与64并发反例已落盘。

## 验证

- `go test ./contract ./mcp ./tests/conformance -run 'MCPCapabilitySnapshotV2|OfficialSDKDiscoveryV1|OfficialGoSDKDiscovery' -count=100`：PASS；
- `go test -race ./contract ./mcp ./tests/conformance -run 'MCPCapabilitySnapshotV2|OfficialSDKDiscoveryV1|OfficialGoSDKDiscovery' -count=20`：PASS；
- `go test ./... -count=1`：PASS；
- `go test -race ./... -count=1`：PASS；
- `go vet ./...`：PASS。

## 当前裁决

- 官方SDK Discovery owner-local状态：`implementation_software_test_yes`；
- 该YES不包含stdio/Streamable HTTP Connect、list_changed调度、受治理`tools/call`、Task/SSE、Credential、production backend/root或SLA；
- 真实`tools/call`仍需Runtime public actual-point authorization projection、Tool exact `MCPExecutionCommandCurrentReaderV1`与宿主composition adapter联合闭合；缺口关闭前真实Provider调用保持零。
