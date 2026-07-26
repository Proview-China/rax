# Review Internal Preview启动诊断

## 事件

2026-07-26，在已合并的安全单节点Internal Preview基础上，选择既有Production Service V1明确的配置、Secret、TLS、SQLite migration/WAL/foreign-key与`integrity_check`作为下一条最短owner-local可运营切片。未新增HTTP公共语义、备份策略或跨Owner合同。

## 产物

- `review-service`新增`PRAXIS_REVIEW_MODE=check`；
- check-only复用真实启动配置与SQLite路径，成功后不创建listener；
- 成功输出固定`praxis.review.service-check/v1` JSON，不含路径、token或Secret，并显式`support_mode=owner-local`、`production_eligible=false`；
- `run-local.sh`在创建DB前拒绝未知mode；
- Internal Preview验证器先执行check-only，再启动真实服务；
- 故障门补充损坏SQLite、未知mode零DB写与无success输出。

## 边界

- check-only对fresh DB会执行schema初始化，因此不是纯只读命令；
- `database=ready`只代表Review-owned单节点启动依赖，不代表跨Owner current、Runtime Gate、Provider或Praxis production readiness；
- 真实Auto Reviewer、外部Provider、OIDC、HA、备份策略、SLA和production composition仍NO-GO。

## 验证状态

- `go test ./... -count=1`：PASS；
- `go test -race ./... -count=1`：PASS；
- `go vet ./...`：PASS；
- `go test ./cmd/review-service -run Check -count=100`：PASS；
- `go test -race ./cmd/review-service -run Check -count=20`：PASS；
- `scripts/test-internal-preview.sh`：check-only、真实服务+SQLite+HTTP+CLI、损坏DB/未知mode与既有封包安全门全部PASS；
- 最终干净提交态确定性双构建、机械检查和Draft PR待本轮收口。
