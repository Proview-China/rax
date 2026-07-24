# ContextCompactionV1首切面完成

时间：2026-07-17 14:00（Asia/Shanghai）

Context Owner内部P3 Compaction/Generation开始落地。首切面新增exact `ContextCompactionSourceRangeV1`、`ContextCompactionSummaryV1`、`ContextCompactionPlanV1`与`ContextCompactionPreparedV1`，以及context-aware `PrepareContextCompactionV1`。

该切面固定：Source Frame Range使用exact refs；Summary的Retained Anchor/Open Effect/Outstanding Work/Uncompressible集合规范排序、唯一且有界；token必须真实缩减；Plan exact绑定Owner Generation current window、Summary与目标RootFrame；候选Generation Parent=expected current Generation、Ordinal严格+1，同输入确定性；Prepare结果显式`Current=false`。

硬反例覆盖expected current/Source Generation/TTL/Ref顺序与重复/cancel漂移，以及“摘要仍提及但Anchor未进入RetainedAnchorSet”必须重新物化。该切面不写Generation current、不调用Runtime Settlement、不写Continuity、不产生外部Effect；S2 current复读、原子Generation current CAS与lost-reply Inspect-only仍是下一切片。

实际验证：

- `go test -count=1 ./contract ./kernel`：PASS；
- `go test -count=100 ./contract ./kernel`：PASS；
- `go test -race -count=20 ./contract ./kernel`：PASS；
- `go test -count=1 ./...`：PASS；
- `go test -race -count=1 ./...`：PASS；
- `go vet ./...`：PASS。

当前Context Go闭集hash=`f33b27175c084ffa461202a4052e71a3fdde58edfb148124d902c6abb5d4f823`。未stage、未commit、未修改其他Owner。
