# Application Component Release V1计划

状态：owner-local实现候选已落盘，等待独立代码审计；production NO-GO。

- [x] 六项共享协调Capability、Port、Factory descriptor；
- [x] Effect/Settlement归Runtime唯一公共Component ID，Application只保留自身coordination cleanup；Runtime dependency/required capability显式闭合；
- [x] reference_only/standalone/production三档truthful publication；
- [x] local/production readiness exact Ref、S1/S2、TTL；
- [x] Catalog lost-reply exact Inspect、64并发与conformance candidate；
- [x] Assembler Repository公开黑盒。
- [ ] durable Command/Outbox、Workflow Journal、Operation Attempt、Run、G6A、Context Refresh、Checkpoint stores；
- [ ] Outbox worker与Recovery worker；
- [ ] Runtime Governance、Run Settlement、Execution Gateway生产接线；
- [ ] Cleanup Owner、production root、Deployment Attestation、独立Certification。

任一局部Coordinator完成均不关闭production P0。
