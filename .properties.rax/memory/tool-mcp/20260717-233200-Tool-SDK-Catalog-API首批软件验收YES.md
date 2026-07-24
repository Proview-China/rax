# Tool SDK与Catalog API首批软件验收YES

## 事件

- 新增Owner-local Go SDK V1：Capability/Tool/Package只提交到Registry `submitted`；exact Inspect绑定ID/Revision/Digest；assembly Resolve和Surface Compile绑定同一Registry Snapshot并执行S1/S2复读；
- SDK主动不暴露Registry Admission/Transition、Provider、Connect、Call、Invoke或Runner handle；
- 新增transport-neutral Catalog API V1：typed cursor绑定Snapshot/filter/last coordinate，支持稳定分页、kind filter、空Registry和跨页漂移拒绝；
- API不选择HTTP/gRPC、Webhook、数据库或消息系统，不导入Application/Harness/Model/Runtime kernel或官方MCP SDK。

## 验证

- `go test ./sdk ./tests/conformance -run 'SDKV1' -count=100`：PASS；
- `go test -race ./sdk ./tests/conformance -run 'SDKV1' -count=20`：PASS；
- `go test ./api -count=100`：PASS；
- `go test -race ./api -count=20`：PASS；
- `go test ./... -count=1`：PASS；
- `go test -race ./... -count=1`：PASS；
- `go vet ./...`：PASS。

## 当前裁决

- SDK/Catalog首批状态：`implementation_software_test_yes`；
- 后续Call/Cancel/Watch、受治理MCP Connect/Call、CLI、Webhook、Package Verify/Fetch与production服务仍未闭合；
- 真实MCP `tools/call`继续受`controlled-mcp-call-port-delta-v1.md`的Prepared-domain-command Association与Runtime public actual-point V3阻断，不能从SDK/API直连Provider。
