# HostV3 可执行参考生命周期 V1

## 状态与结论

- 状态：可执行参考纵切；不是 production composition root。
- 目标：在不制造任何 Owner Current 的前提下，把既有 `StartRequestV3`、原子 Claim/InputV3 sidecar、HostJournalV2 与 HostV3 API 接成可恢复的 `Start → Inspect → Stop`。
- 明确不包含：Console/Page/TS API、Model Loop、Provider、HA/SLA、进程入口、真实七组件 production factories。

## 唯一边界

```text
HostV3
  -> exact Deployment Current read
  -> atomic HostStart Claim + InputV3 sidecar
  -> HostJournalV2
  -> HostV3OwnerPipeline
       -> exact Ready Current Ref
       -> exact Availability Ref
       -> exact CleanupClosure Ref
       -> exact CleanupResult Ref
```

`HostV3OwnerPipeline` 是窄 composition seam，不拥有领域事实，也不得从摘要重建对象。它只能返回真实 Owner 已签发的 exact refs；`HostV3`逐字段验证 Host/Start、Journal、Closure、Ready/Availability 坐标、TTL 与 request digest。

Ready 与 Availability 必须拥有同一个 `ID + Revision + Epoch + ExpiresUnixNano`。二者摘要可以不同，因为它们是不同 Owner 投影；坐标漂移一律 Conflict。

## 恢复与失败语义

- Start/Stop 只有 `UNAVAILABLE` 或 `UNKNOWN_OUTCOME` 才进入原操作 Inspect；Conflict/Invalid/Precondition 直接失败。
- lost reply 后先重读最新 Journal，再 Inspect 原 Start/Stop，禁止用旧 Journal Ref 验证新 Owner projection。
- Inspect 不创建任何事实；Claim过期不删除历史，但返回的 current projection 仍须在自己的有效期内。
- V1/V2/V3共用永久 Claim key；V3 Claim不得进入V2 Journal acceptance，反向同理。
- CleanupClosure 是Stop唯一允许的清理闭包；替换摘要或ID属于splice并Fail Closed。

## 当前缺口

真实 `HostV3OwnerPipeline` 仍待 Application/Runtime/Harness/6+1 Owner composition。现有 HostV2 不能在 V3 Claim 后无损复用：它会重新解释Claim版本、缺少V3 CleanupClosure结果，并且Stop仍接受caller plan。因此本纵切没有伪造V2-after-claim adapter。

黑盒中的reference pipeline使用独立test-only SQLite projection store证明“旧pipeline完全关闭后，新pipeline只读恢复exact projection”。该测试仓不是production Ready/Cleanup Owner，也不得进入非测试composition。

32并发黑盒跨8个Host与8个State Plane SQLite handles，但共享同一进程内reference pipeline mutex；它只证明Host Claim/Journal跨handle线性化，不证明多进程Owner operation只执行一次。production multi-process Owner intent/journal linearization仍为NO-GO。
