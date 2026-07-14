# Harness公共合同与最小可运行骨架v1

状态：已完成（2026-07-14）。计划保留为已执行资产。

## 1. 授权与边界

- 落点：`ExecutionRuntime/harness`独立Go module；
- 允许：公共Schema、Run内Kernel、Context/Model/Event窄Port、Runtime ExecutionPort Adapter、fake与测试；
- 禁止：真实官方/第三方Harness实现、现有model-invoker Adapter修改、生产Scheduler/数据库/RPC、真实模型/Tool/MCP调用和长期明文Secret。

## 2. 实施顺序

1. [x] 固定BootstrapPlan、Manifest、Run State、Event和Completion Claim；
2. [x] 实现单活跃Run Interaction Loop与source sequence；
3. [x] 实现Context/Model/Event窄Port和确定性fake；
4. [x] 实现Runtime ExecutionPort Adapter；
5. [x] 接入Runtime Foundation正常闭环；
6. [x] 增加Action Gateway、取消、背压、配置漂移、Fence和unsupported Checkpoint反例；
7. [x] 运行普通、Race、Vet并同步说明/memory。

## 3. 预期产物

```text
ExecutionRuntime/harness/
  contract/
  ports/
  kernel/
  runtimeadapter/
  fakes/
  tests/
```

产物是可供后续Codex/Claude/ACP或自定义Harness实现的公共骨架，不是生产Harness。

## 4. 完成条件

- 两种Interaction路径通过同一合同；
- Runtime Foundation用真实Harness Adapter替换FakeExecution后跑通；
- Action不会绕过Gateway执行；
- 配置、Intent、Fence、事件、取消和迟到结果fail closed；
- `go test ./...`、`go test -race ./...`、`go vet ./...`通过；
- 中文模块说明、properties索引和memory同步。

以上完成条件均已满足。这里的“完成”只指公共合同与最小骨架，不代表任何具体生产Harness已经实现。
