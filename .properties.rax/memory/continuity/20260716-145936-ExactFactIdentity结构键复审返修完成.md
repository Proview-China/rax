# ExactFactIdentity结构键复审返修完成

时间：2026-07-16 14:59:36 CST

状态：Continuity Checkpoint V2第二轮独立代码复审P1/P2精确返修完成，可交同一独立审计复验。Runtime设计资产P2由Runtime Owner统一纠真，本轮未写Runtime design/plan/memory。

## 返修内容

- `ExactFactRefV2.IdentityKey()`不再返回允许分隔符碰撞的`|`拼接字符串，改为`ExactFactIdentityKeyV2`可比较结构；
- 结构键包含contract、schema、Tenant、Scope、ID、revision、digest及完整`OwnerBinding`七个字段，不再只绑定`Owner.ComponentID`；
- Residual聚合去重、exact ref/Attempt closure canonical排序及Seal-by-Manifest索引统一复用结构键；
- 新增delimiter collision、OwnerBinding任一字段drift、跨Tenant相同Seal ID独立创建/Inspect及禁止串读反例。

## 实际验证

工作目录：`ExecutionRuntime/continuity`

```bash
go test -count=100 ./contract ./domain ./fakes ./storage/memory ./tests/blackbox ./tests/fault ./tests/conformance
go test -race -count=20 ./contract ./domain ./fakes ./storage/memory ./tests/blackbox ./tests/fault ./tests/conformance
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
gofmt -l .
go list -deps ./...
rg -n '"github\.com/Proview-China/rax/ExecutionRuntime/(runtime|harness|application|sandbox|context-engine|tool-mcp|model-invoker|memory-knowledge|review)/' --glob '*.go' .
```

结果：ordinary `count=100` PASS；race `count=20` PASS；full ordinary、full race、vet PASS；`gofmt -l`无输出；依赖仅为标准库和本模块包；禁止跨Owner实现import扫描无命中。

## 边界与移交

- Checkpoint跨Owner集成、production Provider与Restore继续NO-GO；Provider调用数为`0`；
- Partial只诊断，Unknown/lost reply只Inspect，legacy不扩权，不宣称外部世界回滚；
- `.properties.rax/design/runtime/checkpoint-restore-governance-v2/{README,contracts,port-delta,test-matrix}.md` current-truth P2已移交Runtime Owner，避免并发覆盖；
- 未stage、未commit。
