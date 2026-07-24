# Memory/Knowledge Component Release V1候选与production门

2026-07-18，Memory/Knowledge owner-local发布面已落盘：直接复用Agent Assembler、Harness Assembly和Runtime公共类型，以两个独立Owner Capability、Port及Factory descriptor进入Catalog。

发布严格区分reference_only、standalone和production。当前reference Store、内存Owner Store、Context Source Reader与fixture不能证明durable生产能力；durable stores、Credential/current、Purge/Cleanup、Deployment Attestation、独立Certification及production root未闭合，production保持NO-GO。
