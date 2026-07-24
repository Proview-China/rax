# Application Shared Engine Component Release V1候选与production门

2026-07-18，Application owner-local发布面已落盘：直接复用Agent Assembler、Harness Assembly和Runtime公共类型，发布六项共享协调Capability/Port/Factory。

发布严格区分reference_only、standalone和production。局部Coordinator、fakes、owner-local测试与fixture不能成为production证明；durable stores、worker/recovery、Runtime gateways、cleanup、production root、deployment attestation和独立Certification未闭合，production保持NO-GO。

联合审计收口：Effect/Settlement Owner使用Runtime唯一公共Component ID，Application只保留coordination cleanup；Runtime Required Capability与Assembly Dependency均显式fail closed。
