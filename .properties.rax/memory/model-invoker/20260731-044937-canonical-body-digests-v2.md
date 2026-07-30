# 2026-07-31 Model Canonical Body Digests V2

## 粗粒度事件

在最新`origin/main`独立worktree中完成Model Invoker additive canonical body
digest V2最小切片：

- 新增`DigestGovernedModelTurnRequestToolSetV2([]Tool)`；
- 新增`DigestGovernedModelTurnContextBodyV2([]Instruction, []InputItem)`；
- 旧完整`RouteCall`入口先完成Call校验，再委托对应body函数；
- 新旧入口使用相同domain、version、discriminator和canonical body；
- 深拷并按现有Model合同拒绝非法union、strict=false、重复key/name、
  trailing和非法JSON；
- unknown schema/arguments成员不被擅自解释或丢弃，而是作为opaque bytes进入digest；
- Provider、Tool execution、Context Reader调用数均为0；
- 没有Tool、Context、Console实现依赖。

## 验证结果

- focused ordinary `go test ./tests/invocationmaterialv2 -count=100`：通过，
  `247.939s`；
- focused race `go test -race ./tests/invocationmaterialv2 -count=20`：通过，
  `271.366s`；
- Model Invoker full ordinary `go test ./... -count=1`：通过；
- Model Invoker full race `go test -race ./... -count=1`：通过；
- `go vet ./...`：通过；
- `go mod tidy -diff`、`go mod verify`、`gofmt -l`、`git diff --check`：通过；
- `TestInvocationMaterialV2ImportBoundary`：通过。

## 当前边界

Tool adapter尚未实现；Tool Owner仍需在独立slice中把actual
`RouteCall.Request.Tools`交给Model-owned算法。Context helper只公开现有Model
canonical body，不冻结Context Channel/Frame/Material到`Instructions/Input`的全量
lowering。两项跨Owner适配均需独立设计和审计，production继续`NO-GO`。
