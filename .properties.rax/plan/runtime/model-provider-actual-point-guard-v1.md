# Model Provider Actual-Point Guard V1 Plan

状态：owner-local implemented，等待独立代码审计；production NO-GO。

- [x] Additive ports contract，返回 sealed current projection，无 `Allowed`。
- [x] Runtime cancel/current 注入式窄 Reader；不从 Context cancel 伪造事实。
- [x] Kernel 只持 Model boundary、Run lifecycle、Effect、V4 dispatch、control current 的窄读能力。
- [x] 固定 Model boundary → Run → control → Effect → V4 current → Fence 的读取顺序。
- [x] S1/S2 权威水位复读、fresh clock、最小 NotAfter、clock regression。
- [x] effect kind、provider capability、Permit begun、instance Lease/Fence exact 校验。
- [x] public conformance 保持 `ProductionEligible=false`。
- [x] ports/control/kernel/conformance owner-local 单元与 fault 反例。
- [ ] 独立 Runtime 代码审计。
- [x] Runtime Run cancel/current owner-local只读Adapter与软件门禁。
- [ ] Runtime Run/Command生产Owner/Store与composition root。
- [ ] Model/Harness actual-boundary Adapter 与 original-attempt Inspect。
- [ ] 真实 direct/stream/continuation/realtime/raw no-bypass、64 并发 winner 黑盒。
- [ ] production composition root/backend/SLA。
