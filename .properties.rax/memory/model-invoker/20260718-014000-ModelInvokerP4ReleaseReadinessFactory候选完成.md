# Model Invoker P4 Release / Readiness / Factory候选完成

- 时间：2026-07-18 01:40（Asia/Shanghai）
- 范围：仅`ExecutionRuntime/model-invoker`及Model Invoker独占资产。

## 落地产物

- `releasecandidate/`产出Assembler可读的`ComponentReleaseV1`、canonical readiness和conformance；
- 候选固定为`reference_only + production_eligible=false`；
- 11项真实production P0以Required/Missing proof闭包显式保留；
- 发布lost-reply只允许exact read恢复，同ReleaseRef内容漂移立即拒绝；
- import门禁禁止候选依赖`internal`、fake/testkit、Provider和RouteGateway实现。

## 验证真值

- targeted ordinary100：PASS；
- targeted race20：PASS；
- core/layout、vet、gofmt、import、owner-scope diff-check：PASS；
- full ordinary/race：候选及其余包通过，唯一失败为既有`tests/catalogassets/TestDefaultCatalogEvidenceIsCurrentAtWallClock`官方证据wall-clock过期；未修改、未延长证据。

## 裁决

P4 assembly candidate可供Assembler结构验证；Model Invoker production release/readiness仍为NO-GO。Provider配置、内存Store、owner-local测试和fixture都不是production证据，必须关闭11项P0并取得独立deployment certification后才能重新裁决。
