# Harness公共持久接线Owner-local收口

时间：2026-07-23 22:01（Asia/Shanghai）

## 事件

Harness规划内可独立落地、但本身不构成完整production闭环的持久接线已完成：

- Route V2 SQLite Store：Declaration/Conformance/Current、Verified Compile、ActiveRoute current、Wiring与Owner Artifact immutable history/current；
- M2/A2 SQLite Store：verified Assembly与Runtime-neutral Assembly Current immutable history/current；
- Model Prepared ACK SQLite Repository：`ack-id`、`prepared+current`、`prepared-ref`三索引create-once与exact恢复。

共同性质：WAL、foreign keys、FULL synchronous、schema/row/index digest、full-Ref CAS、ABA防护、restart、lost-reply后exact Inspect、S1/S2、fresh-clock TTL、context取消、并发和篡改Fail Closed。Fake/内存Store仍只用于测试。

## 验证

- `go test -count=1 ./...`：PASS；
- `go test -race -count=1 ./...`：PASS；
- `go vet ./...`：PASS；
- `go mod tidy -diff`：零差异；
- gofmt、`git diff --check`、trailing whitespace与生产包import边界：PASS。

冻结实现hash：

- Route Store：`77cc2bbffd3a75ea448b0d56a6f8495c2a4ec64b33d8bc0700e09a5a011581da`
- Route tests：`e6d3c522f14f080ffadc916c30caeb6acff57e52ebfecaba50b43e8962abc8e4`
- M2/A2 Store：`a8ec5b4bc10d3c7a6a172c18c521bf2464f650c19d967e6027663abd1206f960`
- M2/A2 tests：`8910d6fe5bbaa91b1eecbe63349c9c682ecad046e2d6a4f54d5e153edf0b96a0`
- ACK Repository：`3d1692e1d7dae04a972e18996e4b6b0ab26e4cbc499a4f6d1d4a2119421adc69`
- ACK tests：`e4d5f45ba2eebad291e6be2c2c16a6a3329c5ff5969388f04fdaa21c847a3dde`

## 明确未闭

- M2独立Handoff current只有消费接口，没有可从`GenerationID/handoff`无损定位Publication scope/history的Owner backend；不得用sidecar或第二真值补齐；
- Model actual-point Inventory/closed Kind/ACK carrier/Receipt repository与无环Tool Binding current Reader尚未冻结；
- Tool Consumer、Application G6B、Context Continuation、可执行Factory、Cleanup/deployment attestation与production composition root仍未闭合。

因此本事件只把Harness Owner可独立完成的公共持久接线收口，不宣称完整production闭环、production SLA或Capability启用。
