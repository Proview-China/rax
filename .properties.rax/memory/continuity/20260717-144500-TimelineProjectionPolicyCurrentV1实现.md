# Timeline Projection Policy Current V1实现

时间：2026-07-17 14:45 CST

## 当前真值

C-01先前由测试fake提供的Projection Policy current已收回Continuity Owner：

- opaque Policy ID、revision、ScopeDigest与canonical digest组成exact ref；
- `active|revoked|expired`为currentness闭状态，不新增业务Policy结论；
- create-once、exact history Inspect、subject current Inspect与expected-ref CAS；
- same-ID exact create幂等，换内容Conflict；旧revision历史可读但ValidateCurrent失败；
- stable sealed Checked/Expires/Digest，fresh Validate在`now == expires`边界Fail Closed；
- 64路不同内容CAS只有一个赢家，progressed replay不形成ABA；
- 同Policy ID跨Scope独立，不串读；lost create/CAS reply通过exact Inspect收敛；
- `RequestedNotAfter`不进入Policy projection或digest，只由Continuity Adapter在全部自然TTL之后最终截短。

Adapter测试已从静态Policy fake切换为真实内存Repository；计数wrapper仅验证S1/S2/fresh调用次数。

## 实际验证

```bash
go test ./contract ./storage/memory ./runtimeadapter -run 'TestTimelineProjectionPolicy|TestTimelineProjectionAdapter' -count=100
go test -race ./storage/memory ./runtimeadapter -run 'TestTimelineProjectionPolicy|TestTimelineProjectionAdapter' -count=20
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
gofmt -l .
```

结果全部PASS；`gofmt -l`无输出。

本实现仍是内存reference repository，不声明production durability/SLA，也不解锁真实typed Owner Reader、Application root、Checkpoint或Restore。
