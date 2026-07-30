# Context Model Input Lineage Current V1 实施计划

## 范围

仅在Context Engine内新增additive contract、ports、kernel reader、线程安全测试
fixture及unit/blackbox/conformance/failure测试。现有Material/Frame/runtimeapi
合同保持不变。

## 文件落点

- `contract/model_input_lineage_v1.go`
- `ports/model_input_lineage_v1.go`
- `kernel/model_input_lineage_v1.go`
- 对应`*_test.go`
- `internal/testfixture/model_input_lineage_v1.go`
- `tests/{blackbox,conformance,failure}/model_input_lineage_v1_test.go`

## 验收

- Material/Frame不同Digest正例；
- Owner/Kind/ID/Revision/Digest逐轴漂移和互换失败关闭；
- Material exact与current必须一致，history-only拒绝；
- Material和Frame均完成S1/S2复读；
- request/Material/Frame/maxTTL逐项最小值；
- clock rollback、TTL crossing、typed nil、cancel、Unavailable返回零projection；
- 64并发Frame flip不产生拼接projection；
- 旧Material exact ref不能接受重新密封后的不同内容；
- 负例在可预检阶段不得调用下游Frame reader。

验证门为定向count100、race count20、模块完整ordinary/race、`go vet ./...`、
`gofmt`、`git diff --check`和路径范围检查。未提供production Frame backend，
因此本计划完成后仍不得启用Harness/Model/Provider dispatch。
