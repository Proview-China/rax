# Governed Model Turn V3 Actual Boundary 施工中

## 当前事实

- worktree已rebase基线：`origin/main@5e491e9450b76a3f488cf14703b9d4396dc670bc`
- 分支：`agent/governed-model-turn-v3-routegateway-actual-boundary`
- V3 routegateway pre-invoke路径已形成未提交候选。
- authoritative Context/Tool pair会在S1/S2各读取一次，并由同次读取同时产生Authorization与完整projection；非min source TTL/Checked漂移不会被二次读取TOCTOU或聚合最短TTL掩盖。
- Commit ACK在S1/S2按完整body与currentness复检；exact Turn保持Ref的body splice也会Fail Closed。
- 现有`SecretResolver`无法证明同version/expiry下的material generation exact current，因此本切片在prepare后、Boundary前Fail Closed。
- caller Boundary exact匹配/冲突、source TTL缩短/延长与Checked漂移、ACK expiry/body漂移、Turn body splice、S2 clock regression、Runtime draft preflight与Provider lease release error chain均已补门禁。
- adapter lease release/retire改为顺序无关终态，stale且refs为零时Closer恰一次。
- 已有64并发、same-coordinate material drift与重放门禁测试，Boundary/Provider均为0。

## NO-GO

尚无credential exact current/handle/reader、实际Provider request→`ActualProviderInjectionDigest`公开canonical，也无V3 Provider Attempt/Observation持久化与exact Inspect；当前仅证明Boundary/Provider/Observation=0的pre-invoke reference路径。
