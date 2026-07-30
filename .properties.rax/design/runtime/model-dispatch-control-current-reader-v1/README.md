# Model Dispatch Control Current Reader V1

状态：Runtime owner-local 实现与软件门禁通过，等待独立代码审计；production composition NO-GO。

## 目标

`ModelDispatchControlCurrentReaderV1`把 Runtime 已有的 Run、Desired State 与 Command 历史只读事实，封成 Model Provider actual-point guard 所需的短 TTL current projection。它不读取 Context cancel，不持有命令或Run写权，不接触 Model、Harness或Provider。

## 只读依赖

- `ModelDispatchRunFactCurrentReaderV1.InspectRun`
- `ModelDispatchCommandFactCurrentReaderV1.ReadDesiredState`
- `ModelDispatchCommandFactCurrentReaderV1.ListCommands`

现有`RunFactPort`与`ApplicationCommandFactPortV2`可通过Go structural typing满足这些窄接口，但构造器只返回单方法`ModelDispatchControlCurrentReaderV1`，不会保留mutation capability。

## 派生闭集

| 精确事实 | 输出 |
|---|---|
| Run=`running`，Desired=`running`，initial或Start/Resume/Input/Approve/Deny current command | `dispatchable` |
| Run=`running`，Desired=`running`，LastCommand=`cancel_run` | `cancel_requested` |
| Run=`running`，Desired=`fenced`，LastCommand=`fence` | `fenced` |
| Run=`running`，Desired=`fenced`，LastCommand=`revoke` | `revoked` |
| Stop、Run stopping/terminal、command superseded/invalidated/indeterminate | `indeterminate` |

Desired revision大于一时必须绑定唯一LastCommand；LastCommand必须与Desired revision、ExecutionScope、preconditions、recorded time精确一致。缺失、重复、错scope、错revision、事实无法判定均Fail Closed，不生成projection。

## Current边界

1. S1读取Run、Desired、LastCommand；
2. S2按同序复读并比较canonical source watermark；
3. fresh clock不得回拨；
4. seal后的`NotAfter=Checked+1s`；
5. Runtime actual-point guard仍会再次双读本Reader。

该TTL只是强制重新读取的最大缓存窗，不把Command提交时的Envelope TTL重新解释为持续授权。

## 当前限制

- `runtime/storage/sqlite`尚无生产Run/Command owner实现或composition root；
- 当前只有reference fake可满足完整读依赖；
- 本Reader不证明Host availability、Model Boundary CAS winner或Provider no-bypass；
- `ProductionEligible`保持`false`，不声明durability、availability或SLA。
