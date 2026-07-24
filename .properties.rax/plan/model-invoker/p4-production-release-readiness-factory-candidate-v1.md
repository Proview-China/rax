# Model Invoker P4 Production Release / Readiness / Factory Candidate V1 Plan

- [x] 核对Prepared、Route/Profile/Registry/Provider current、CommitGate ACK与Harness bridge公共边界；
- [x] 冻结 `reference_only` 候选与真实P0，不允许fixture/Provider配置冒充production；
- [x] 实现Owner-owned release candidate、canonical readiness与conformance；
- [x] 覆盖TTL、clock rollback、proof drift、descriptor drift、typed nil、lost reply exact recovery和并发；
- [x] targeted ordinary100/race20、gofmt/import/vet/diff；
- [ ] full ordinary/race：候选及其余包均通过，现仅被既有 `tests/catalogassets/TestDefaultCatalogEvidenceIsCurrentAtWallClock` 的官方证据过期阻断；禁止篡改或延长证据以伪造全绿。

## 实测裁决（2026-07-18）

- `go test -count=100 ./tests/releasecandidate`：PASS；
- `go test -race -count=20 ./tests/releasecandidate`：PASS；
- `go test -count=1 ./tests/releasecandidate ./tests/core`：PASS；
- `go vet ./...`、`gofmt -d releasecandidate tests/releasecandidate`、owner-scope `git diff --check`：PASS；
- `go test -count=1 ./...` 与 `go test -race -count=1 ./...`：候选通过，唯一失败为上述既有Catalog wall-clock证据过期门禁。

完成本计划只表示P4 assembly candidate可供Assembler验证。所有P0关闭并经独立production certification之前，Model Invoker production release/readiness仍为NO-GO。
