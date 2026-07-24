# Tool/MCP Component Release V1候选与production门

2026-07-18，Tool/MCP owner-local发布面已落盘：直接消费Agent Assembler、Harness Assembly与Runtime公共合同，提供两项Capability、两个effectful Port、两个Factory descriptor及exact readiness。

发布显式区分：无owner-local证明为`reference_only`；G6A/Surface/Binding/Controlled Provider/MCP owner-local current闭合为`standalone`；只有durable store、Credential、真实transport/current、actual-point、MCP lifecycle/Inspect、cleanup、deployment attestation和独立Certification全部闭合才是`production`。

当前production门未闭合。内存Store、测试Provider、official SDK in-memory Session与test composition不能作为生产证明。
