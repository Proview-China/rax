# Memory/Knowledge Component Release V1计划

状态：owner-local与reference发布已完成；Production closure V2按`non_ha`/`ha`双Profile进入实现。实际部署资源与外部Owner current未注入前production仍NO-GO。

- [x] 直接复用Assembler/Harness/Runtime公共Release、Descriptor和Manifest类型；
- [x] Memory与Knowledge两个Capability、effectful Port和Factory descriptor；
- [x] reference_only/standalone/production三档truthful publication；
- [x] local/production exact readiness、S1/S2、TTL、lost-reply Inspect与64并发；
- [x] conformance candidate和Assembler Repository黑盒。
- [ ] 真实durable Memory/Knowledge fact/content stores；
- [ ] Authority/Policy/Credential生产current；
- [ ] 生产Retrieval Index、Context Source、Settlement与Purge Effect；
- [ ] Cleanup Owner、Deployment Attestation和独立Certification；
- [ ] production composition root注册Factory与后端。

未完成项不阻塞非生产Catalog候选，但严格阻塞production。

## Production closure V2实施清单

- [x] 在`ExecutionRuntime/memory-knowledge/production`落`ProductionProofBundleV2`、双Profile、canonical seal/validate；
- [x] 复用Runtime公开`ResourceCurrentReaderV1`，S1/S2 exact检查BindingSet与四个durable Handle；
- [x] 将opaque外部Owner current保持为Ref，不复制或解释其事实；
- [x] 将验证后的bundle无损映射到现有`release.ProductionReadinessProjectionV1`；
- [x] 覆盖non-HA、HA、TTL、clock rollback、cancel、role/kind、S1/S2 drift与64并发；
- [x] ordinary count=100、race count=20、full ordinary/race/vet、gofmt、diff与import boundary；
- [x] 同步module/memory current truth。

退出分两轴：

| 轴 | 退出条件 |
|---|---|
| Owner software | 上述实现与测试全部通过，可标记`implementation_software_test_yes` |
| Deployment production | 对应环境实际提供全部exact resources/current、独立Certification且Catalog发布`SupportProductionV1`；未提供时继续NO-GO |

可并行：四个durable Resource Owner、Authority/Policy/Credential Owner、Retrieval/Context/Settlement/Purge/Cleanup Owner、Deployment与Certification Owner分别发布事实。必须串行：外部事实完成 -> bundle S1/resource Inspect/S2 -> release Publisher S1/S2 -> Catalog Ensure/Inspect。
