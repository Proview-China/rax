# Runtime线性化控制事实与IdentityLease机制落地

## 事件

2026-07-14，在用户把Harness共用事实工程转入独立任务后，当前Runtime任务继续只推进组件中立机制。本次完成Command、Desired State、Outbox和IdentityExecutionLease抽象事实合同及确定性内存fake，没有接入Harness或其他真实组件。

## 本次产物

1. `CommandFactPort`把CommandRecord、Desired State和Outbox固定为一次逻辑提交；
2. 内存fake实现revision CAS、命令安全支配、精确幂等范围、Payload Digest冲突和Outbox派发确认；
3. Evidence不可用时命令不接受，Desired State、Command和Outbox均保持零部分写入；
4. `IdentityLeaseFactPort`实现同一Identity的单reserved/active持有者；
5. Lease reserve产生更高identity epoch，reserved只表达排他占位，activate后才获得一般执行权；
6. Lease renew、revoke、release、expiry和替换继续使用revision CAS；active Lease不能绕过撤销直接释放；
7. 并发命令、双主Lease、幂等重放、幂等Payload漂移、证据故障、Stop支配、续租旧revision和过期替换均有自动化反例。

## 验证

- `go test ./...`：通过；
- `go test -race ./...`：通过；
- `go vet ./...`：通过。

## 保持边界

- 未修改现有`model-invoker`；
- 未实现或接线Harness、Context/Cache、Tool/MCP、审批链和Sandbox；
- 内存fake只用于确定性合同与故障验证，不是生产数据库选择；
- 下一切面进入无循环Admission/Activation及逐中断点恢复。
