# PreparedModelInvocation M0/M1主审P1返修候选完成

## 时间

2026-07-16 23:07:00 +0800

## 粗粒度事件

Model M0/M1主独立审计结论为`NO(P0=0/P1=3/P2=0)`。本轮只返修Model自有Prepared Invocation Repository与preparedinvocation测试，没有扩展M2，没有修改Runtime、Harness、Tool、Application或production root，也没有stage或commit。

返修后，同sealed atomic Ensure第一次返回Indeterminate时仍只允许恢复一次；第二次返回Conflict、Unavailable或Indeterminate时，最终错误类型只取第二次结果，不再通过`errors.Join`把首次Indeterminate泄漏为最终分类。public Historical/Current Ensure会在任何Repository调用前检查nil/canceled context，typed-nil Repository固定归一为Invalid。内存Exact Reader在取得读锁后与释放读锁后都复查context，取消后不得成功返回。

新增反例覆盖Historical/Current第二次Ensure三类结果、public/store ensure/read的nil与canceled context、读锁内及解锁后取消、64并发同identity异content唯一canonical赢家、stored canonical wire篡改重算拒绝，以及Ensure/read返回值污染不影响后续exact读取。

## 验证

- `go test ./tests/preparedinvocation -count=100`：通过；
- `go test -race ./tests/preparedinvocation -count=20`：通过；
- `go test ./...`：通过；
- `go test -race ./...`：通过；
- `go vet ./...`：通过；
- `gofmt -d`：无差异；
- `git diff --check`及直接trailing-whitespace/untracked扫描：通过；
- import/type-identity conformance：通过。

## 当前边界

- 两次独立短审均为`YES(P0=0/P1=0/P2=0)`，Model M0/M1返修切片已接受；
- 当前只解锁Harness M2；Model M2-M5、Tool与production root继续NO-GO，没有获得授权或实现；
- 内存Store仍只是reference Fake/Conformance，不是生产driver、composition root、retention service或SLA。

## 关联资产

- [实施计划](../../plan/model-invoker/prepared-model-invocation-pre-dispatch-gate-v1.md)
- [模块入口](../../module/model-invoker/README.md)
