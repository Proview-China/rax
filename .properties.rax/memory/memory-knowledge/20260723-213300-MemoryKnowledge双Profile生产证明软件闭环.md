# Memory/Knowledge双Profile生产证明软件闭环

时间：2026-07-23 21:33 +08:00

## 结果

Memory/Knowledge新增backend-neutral `production`包，状态为`implementation_software_test_yes`。它不预选数据库、RPC、进程拓扑或SLA，也不创建其他Owner事实。

- `non_ha`：只接受单写者Fence、恢复证明、备份恢复证明，Replica/WriteQuorum/ReadQuorum固定为1，禁止HA证据与HA声明；
- `ha`：要求ReplicaCount >= 3、WriteQuorum为多数、ReadQuorum有效，并具备复制current、仲裁current、故障转移证明与单调current证明；
- 四个durable Memory/Knowledge fact/content Resource Handle复用Runtime公开`ResourceCurrentReaderV1`执行BindingSet和Handle exact Inspect；
- Authority/Policy/Credential、Retrieval/Context、Settlement/Purge/Cleanup、Deployment与Certification保持opaque Owner current Ref；
- 固定执行Bundle S1 -> Resource exact Inspect -> Bundle S2 -> existing Release readiness映射；漂移、过期、clock rollback、cancel、role/kind错误、缺资源均Fail Closed。

## 验证

- `go test ./production -count=100`：PASS；
- `go test -race ./production -count=20`：PASS；
- `go test ./...`：PASS；
- `go test -race ./...`：PASS；
- `go vet ./...`：PASS。

## Current truth

Owner软件侧生产退出验证已闭合。实际durable资源、外部Owner current、Deployment Attestation与独立Certification仍必须由对应环境提供；未注入并经Catalog发布`SupportProductionV1`前，deployment production保持NO-GO。
