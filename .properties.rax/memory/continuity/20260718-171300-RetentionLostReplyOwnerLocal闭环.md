# Retention lost-reply owner-local闭环

时间：2026-07-18 17:13 CST

## 完成内容

- `RetentionManager.Create`在durable write回包未知时只Inspect原Object ID；仅当完整Fact exact相等才返回成功，同ID换Policy/Classification/时间内容返回revision conflict。
- `RetentionManager.Transition`在CAS回包未知时只Inspect原Object ID；仅当revision/state/evidence/time完整Fact exact相等才收敛，不重放CAS、不形成ABA。
- Constructor拒绝typed-nil Store/Clock；Create在backend读取前先验证Fact。
- SDK `InspectRetention`复验返回Fact完整合法性，错误Reader Fail Closed。
- `PhysicalPurge`继续明确unsupported；本切片不选择Retention Policy、远程Provider或SLA。

## 实际验证

- `go test ./domain ./sdk -run 'Retention|ClientFailsClosed' -count=100`：PASS。
- `go test -race ./domain ./sdk -run 'Retention|ClientFailsClosed' -count=20`：PASS。
- `go test ./tests/fault -run Retention -count=100`：PASS。
- `go test -race ./tests/fault -run Retention -count=20`：PASS。

## 保留边界

- Legal Hold/Privacy Erasure Policy仍由外部权威Policy Owner提供；
- Physical Purge、remote archive/delete、KMS与deployment root仍未实现；
- Fake只用于故障注入，不声明生产Backend或SLA。
