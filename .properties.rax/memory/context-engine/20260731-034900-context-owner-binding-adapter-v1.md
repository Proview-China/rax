# Context Owner Binding Adapter V1 实现候选

## 结果

基于最新`origin/main@8969b2159ea646e202f25e49f942ed12a028ad8b`，在独立
detached worktree完成Context Owner的authoritative
`ContextModelInputLineageCurrentV1 -> InvocationContextOwnerBindingProjectionV1`
适配器候选，当前保持未提交等待独立审计。

已落地：

- Model request只能映射为Context固定
  `praxis.context/model-input-material` exact source；
- 同一Context lineage current reader完整S1/S2复读；
- Owner `{ComponentID, BindingDigest}`、Material/Frame两个Kind和全部exact coordinate
  的完整验证与无损映射；
- Model公开owner映射/sealer派生neutral Owner；
- `ContextLineageDigest`直接绑定Context authoritative projection digest；
- fresh clock、最小expiry、`now == expiry`和clock rollback失败关闭；
- typed nil、Unavailable、Unknown、cancel、history-only/current=false均零projection；
- 64并发稳定窗口一致，64并发flip窗口全部失败关闭；
- production source import边界不含Model实现子包、Provider、Harness或Tool。

早审P1已原地修复：删除未冻结的Owner Binding、Material、Frame、Lineage四digest
全不等约束。`Owner.BindingDigest == Material.Digest`现有明确正例并保持完整raw
Owner；Material/Frame角色继续只由公开Kind与exact coordinate区分。

## 验证

- `go test -count=100 ./modelinvokeradapter`：通过；
- `go test -race -count=20 ./modelinvokeradapter`：通过；
- `go test -count=1 ./...`：通过；
- `go test -race -count=1 ./...`：通过；
- `go vet ./...`：通过；
- `go mod tidy -diff`、`go mod verify`：通过；
- gofmt、`git diff --check`、import boundary：通过。

## 边界

未修改Model、Runtime、Harness、Tool、Sandbox、Application、Host或Provider，未新增
Store或production root，未写Context/Model事实，未调用Provider。

发布前已rebase到`origin/main@909de45f`，包含独立合并的Context Frame PR #57。
durable Frame current reader继续通过既有lineage窄Reader边界与adapter共存，不引入
第二套Frame语义；blackbox fixture已按显式同一Owner绑定更新。

Context testfixture仍只提供reference黑盒验证，不是production composition。
owner-local/reference及durable reader共存验证已完成；RouteCall lowering与Harness
production composition仍未落地，dispatch继续`NO-GO`。
