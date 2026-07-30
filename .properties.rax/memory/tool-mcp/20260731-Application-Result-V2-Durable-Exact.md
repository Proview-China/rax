# 2026-07-31 Application Result V2 Durable Exact

- caller-supplied whole-fact Ensure/mirror 方案已撤销，未保留旁路写 API。
- Application V2 result 由现有 Adapter close create-once seam 写入 SQLite。
- original Application result ref 可读取同一 canonical request/result row；64 并发、restart、splice 与 lost reply 定向门通过。
- exact read 不触发 Tool execute 或 Provider。
- 完整 B1 仍 BLOCKED：真实 Tool V2 fact 与旧 Domain fact 不可混用，且 production 无 Settled projection producer/store/current reader。
