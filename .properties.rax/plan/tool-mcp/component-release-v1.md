# Tool/MCP Component Release V1 落地计划

状态：owner-local实现候选已落盘，等待独立代码审计；production NO-GO。

## 已完成

- [x] 直接复用Assembler `ComponentReleaseV1`、Harness assembly descriptors与Runtime Manifest V2；
- [x] `reference_only`、`standalone`、`production`三档truthful publication；
- [x] Tool G6A与MCP官方SDK两个Capability/Port/Factory descriptor；
- [x] local/production readiness exact Ref、canonical digest、TTL、S1/S2；
- [x] Catalog create-once、lost-reply exact Inspect、64并发；
- [x] conformance candidate与公开Assembler Repository黑盒测试。

## 仍未完成

- [ ] 真实Host durable State Plane与四类production store；
- [ ] Credential Broker与短期credential current；
- [ ] production Provider transport/current、actual-point部署证明；
- [ ] MCP真实transport lifecycle/Inspect与cleanup；
- [ ] deployment attestation及独立Certification Fact；
- [ ] production composition root注册Factory与transport；
- [ ] 独立代码审计与最终release enablement。

上述未完成项不阻塞`reference_only/standalone`进入Assembler Catalog，但严格阻塞`production`。

