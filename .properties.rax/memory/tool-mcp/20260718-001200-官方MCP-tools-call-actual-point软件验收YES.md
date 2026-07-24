# 官方MCP tools/call actual-point软件验收YES

时间：2026-07-18 00:12（Asia/Shanghai）

## 事件

Runtime public `ControlledOperationPhysicalExecutionAuthorizationV3`/
`ControlledOperationPhysicalExecutionPortV3`与Prepared-domain-command Association已落盘。
Tool/MCP实现了`MCPExecutionCommandFactV1`/current Reader、initialized official SDK
Session exact绑定、create-once physical admission、真实in-memory `CallTool`与
`MCPProtocolReceiptV1`。

## 验证

- Runtime ports定向ordinary×100、race×20、定向full ordinary/race与vet通过；
- Tool MCP Call定向ordinary×100、race×20通过；
- Tool module `go test ./... -count=1`、`go test -race ./... -count=1`、
  `go vet ./...`与gofmt检查通过；
- 覆盖64同key单effect、lost provider reply只Inspect、Association/Provider/Command
  漂移、TTL crossing、clock rollback、typed-nil/canceled context和official SDK真实调用。

## 边界

状态为`implementation_software_test_yes`，只覆盖owner-local actual-point第一切片。
Tool Protocol Receipt仍只是Observation。live Runtime Evidence Gateway与Observation
Gateway可复用，但缺少Application-facing窄协调Port来原子分配Evidence source sequence、
append governed Evidence并记录正式Provider Observation；Tool不得伪造
`EvidenceRecordRefV2`。Runtime V3 issuer/root、持久State Plane/Session、stdio/HTTP
Connect、Credential、DomainResult/Settlement、G6B与production能力继续NO-GO。
