# Tool G6A Model/PendingAction Payload绑定修复

时间：2026-07-16 12:47（Asia/Shanghai）

中央G6A合同地图发现Model唯一Call的`CanonicalArgumentsDigest`与Application/Harness `PendingAction.PayloadDigest`未建立强制等式，存在把合法Model call与任意PendingAction payload拼接后执行的风险。

依据Tool design V1既有语义，本轮裁决为直接exact equality：PendingAction payload就是Model唯一Call的canonical arguments，Application不得重写参数。因此`SingleCallCanonicalCommandV1.Validate`和Application `canonicalCommandV1`现强制`CanonicalArgumentsDigest == PayloadDigest`，Candidate继续强制`Candidate.Payload.ContentDigest == PendingAction.PayloadDigest`。V1不存在隐式schema transformation；未来若需要转换，必须由绑定Settlement Owner产出typed Transformation/Association Fact并发布只读Port Delta，不得兼容放宽当前合同。

`ApplicationG6AFixture`已改为从Model canonical arguments计算PendingAction payload digest，不再制造不同digest。新增合同层与真实V2 Owner flow拼接攻击反例：不相等时只允许一次Model exact Reader读取，零Watermark、零Candidate、零Runtime Gateway Entry/admission、零V1 Provider、零DomainResult/Settlement。

本事件不改变Runtime V2、Owner/Settlement边界、Unknown inspect-only或G6B production enable门。

## 验证

- `go test ./contract ./action ./applicationadapter -count=1`：PASS。
- `go test -count=100 ./action ./applicationadapter ./runtimeadapter`：PASS。
- `go test -race -count=20 ./action ./applicationadapter ./runtimeadapter`：PASS。
- `go test ./... -count=1`：PASS。
- `go test -race ./...`：PASS。
- `go vet ./...`：PASS。
- `test -z "$(gofmt -l .)"`：PASS。
- Runtime/Application Adapter定向Conformance、import boundary及production zero-network扫描：PASS。

当前结论仅覆盖G6A隔离实现与测试；仓库仍无production composition root、Provider backend或生产能力启用，G6B门禁不变。
