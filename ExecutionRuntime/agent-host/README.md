# Praxis Agent Host

`agent-host` 是 Praxis 唯一进程级 Composition Root。当前 H1-H2 已完成 Host 自有公共合同、窄依赖反转 Port、exact Factory Registry、编排 Journal、确定性 DAG 构造与 reverse-DAG cleanup；H3 第一纵切已接通真实 Definition current、Agent Assembler 与 Harness Compiler owner adapter。

## 当前可用范围

- `contract`：HostConfig、Host API DTO、exact refs、Factory key、Construction Graph、Journal 与严格 `SystemReadyV1`；
- `ports`：Host 自有 Decoder/Assembler/Compiler/Binding/Readiness/Journal/Factory 窄接口；
- `registry`：构建时 exact 注册、alias/duplicate/drift fail closed、seal 后只读；
- `journal`：create-once、revision CAS、Binding/Construction deterministic attempt write-ahead、lost-reply exact Inspect 恢复；
- Review→Model 关联：Host-owned 中立 Review Attempt exact coordinate、公开 Governed Model command 的 create-once 关联、append-only history/full-ref current CAS、S1/S2 与内存/SQLite reference persistence；
- `composition`：稳定拓扑构造、Factory canonical start-or-inspect、unknown outcome 持久化、reverse-DAG cleanup；
- `lifecycle`：Validate/Assemble/Start/Inspect/Stop 共用入口，按 `HostID + StartID` 分片串行化；只有 Ready phase、fresh validation、exact ReadyRef 与全 6+1 production readiness proof 同时成立才返回 Ready。

Binding 或 Construction 的 planned/unknown attempt 都是未完成的外部 Effect 证明。重试只能复用原 AttemptID 和 request digest；Stop/Reconcile 即使清理完全部已知 handle，也不得把仍有未决 attempt 的生命周期写成 Closed。

Journal successor 只允许 BindingAttempt 首次以 planned 写入、ConstructionAttempt 每次在尾部追加一个 planned；禁止直写 bound/constructed/unknown 或批量 attempt。Factory 已返回的有效 handle/ref 会先进入确定 cleanup 集，再持久化 constructed progress。Create/CAS panic 视为 unknown reply并立即 Inspect exact desired；Port panic 不得越过 Host API。

## 尚未实现

H3 后续 Binding/Generation Association、H4 Runtime/Application/Harness production 接线、6+1 executable production factories、H5 CLI、真实 backend 与系统启动仍未实现。当前各 Owner 的 Release/Readiness 多为 `reference_only`、`assembly_candidate` 或 `standalone`；ModuleFactoryDescriptor 只是声明，不是可执行 factory。Host-owned 窄接口不会把 fixture、descriptor 或缺失接线冒充生产能力。

Review→Model 关联当前同样没有 production composition root，也不执行模型、不创建 Runtime Fact。Review-owned adapter 仍须把 Review Attempt exact Ref 逐字段映射为 Host 中立 coordinate；Host 不导入 Review 实现或合同。

生产代码禁止导入 Runtime `foundation`、任意 `fakes/internal/testkit` 和裸 Provider SDK。Factory constructor 不得隐藏 Provider、网络或不可逆资源 Effect。

## 验证

```bash
go test -count=100 ./...
go test -race -count=20 ./...
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
```
