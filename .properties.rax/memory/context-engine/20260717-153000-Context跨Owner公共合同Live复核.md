# Context跨Owner公共合同live复核

时间：2026-07-17 15:30（Asia/Shanghai）

Context Owner在完成第六Offline Cache Inspect入口后，只读复核Application、Harness、Model Invoker与Continuity live公开合同，并新增业务终点覆盖矩阵。

- Application仍只有SingleCall Tool Action V1/V2，没有Context Refresh三段Port或SettledAction Context Source Reader；
- Harness `ModelTurnCandidateV2`仍只有`ContextRef + ContextDigest`，`ContinuationRefV2`不携new exact Frame/Manifest/Generation binding；
- Model Invoker `union.ContextReference`仍缺Revision/Digest/Expiry/current projection；
- Continuity typed Owner-current Reader/Projection/Router仍位于`continuity/runtimeadapter`，未发布到public ports；
- 因此没有合法的production Context cross-owner Adapter或composition root可实现。Context没有创建私有桥、复制nominal、导入其他Owner实现或改写对方目录。

新增`.properties.rax/plan/context-engine/coverage-matrix.md`逐项区分Owner-local完成、离线合同完成、cross-owner缺口和production NO-GO。PromptAsset权威形态仍等待用户裁决，未写Go。
