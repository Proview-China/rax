# Harness Run内Kernel与Interaction Loop

## 1. 边界

Harness Kernel只拥有当前Run内Interaction Loop和Session状态；Runtime拥有Run Record、Instance生命周期和Execution Outcome。Kernel不得直接修改Runtime Aggregate。

## 2. Run内状态机

```text
idle
  -> running
     -> waiting_input -> running
     -> waiting_action -> running
     -> cancelling
  -> terminal
```

- `waiting_action`必须保存唯一ActionRef；重复或错误ActionResult拒绝；
- V1每个Harness Endpoint最多一个活跃Run；终态不可复活；
- 每次状态更新使用revision，事件使用独立单调source sequence；
- Cancel为单调安全动作；迟到Model Turn结果不能覆盖`cancelling/terminal`；
- Completion只形成Harness Claim，不直接写Runtime成功。

## 3. Interaction Loop

```text
StartRun
-> ContextPort.Prepare
-> persist run_started/model_turn_started candidates
-> ModelTurnPort.Invoke(intent + fence)
-> output completed
   or action_requested(waiting_action)
   or input_requested(waiting_input)
-> receive reviewed ActionResult/Input with fresh model intent
-> next Model Turn
-> completion claim
```

Harness不在`action_requested`后直接调用工具；这保证Tool/MCP与Review Gateway可在后续组件接入时保持唯一入口。

## 4. 取消、错误与背压

- Context取消必须传播到正在进行的Model Turn；
- 取消后迟到结果只保存为迟到Observation，不恢复运行；
- Event Candidate持久化失败时，下一外部Model Turn不得派发；
- 外部调用回包不明时Run Claim为indeterminate，并交由Runtime Effect恢复；
- 事件队列、Payload、迭代次数和并发必须有显式上限；首切面使用测试配置，不写死生产SLA。

## 5. Checkpoint

Checkpoint只有Manifest明确声明并存在可恢复Session事实时才允许。首个骨架不承诺跨Run/跨Instance Session，因此Descriptor不暴露checkpoint能力，并返回稳定unsupported结果。
