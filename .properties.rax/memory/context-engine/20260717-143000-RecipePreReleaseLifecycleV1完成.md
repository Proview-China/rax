# Recipe pre-release lifecycle V1完成

时间：2026-07-17 14:30（Asia/Shanghai）

Context Owner新增不可变Recipe注册与`draft -> validated -> evaluated -> review_pending|rejected`生命周期。每一步形成独立rev1 Lifecycle Fact并绑定previous exact ref；lifecycle head通过expected exact CAS推进，历史Fact不可修改。同一head的64个并发后继只有一个赢家，lost reply通过Inspect head恢复，cancel保持零head。

该head只是Context开发流程状态，不是production Recipe current binding。`publish/rollback/revoke`明确返回`ErrUnsupported: CTX-D07`；实现没有普通Review FactRef type-pun、没有复用Run内V4/上游Settlement、没有Runtime Effect或production current写入。`releasestore.Memory`只是进程内参考Store，不是State Plane或SLA。

实际验证：

- `go test -count=100 ./contract ./kernel ./releasestore`：PASS；
- `go test -race -count=20 ./contract ./kernel ./releasestore`：PASS；
- `go test -count=1 ./...`：PASS；
- `go test -race -count=1 ./...`：PASS；
- `go vet ./...`：PASS。

当前Context Go闭集hash=`13ea113094a728d27e67da3c5338f94622063250c33b269de31f11f420659f9c`。未stage、未commit、未修改其他Owner。
