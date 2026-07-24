# Organization Release Clock Cursor P0修复

## 事件

独立代码审计发现Publisher所有fresh检查只与初始时间比较，无法拒绝`t0 -> t5 -> t3`的请求内中途回退。Organization Owner已将时钟改为每次成功观测后单调推进的请求内cursor；任何后续观测早于cursor都返回`ReasonClockRegression`。

## 反例

新增`TestClockCursorRejectsMidPublicationRegressionBeforeCatalogWrite`：S1在`t5`成功后，S2观测`t3`必须Fail Closed，catalog commit计数保持0。cursor只存在于单次Publish调用内，不扩大production事实或跨请求创建第二时钟Owner。

## Fresh验证

```text
go test -count=100 ./release       PASS (1.471s package time)
go test -race -count=20 ./release PASS (3.383s package time)
go test -count=1 ./...            PASS
go test -race -count=1 ./...      PASS
go vet ./...                      PASS
gofmt                             PASS
```

Production readiness、executable factory、Review variant与Host root仍为NO-GO。
