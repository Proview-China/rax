# ContextOfflineIngressV1完成

时间：2026-07-17 14:40 CST

Context Owner已完成独立只读开发者入口：SDK公开四类request Seal/Encode与四组wire cap；`offlineapi.ContextOfflineAPIV1`提供四typed方法和严格JSON dispatch；`cmd/context`提供`recipe validate|compile|preview`与`frame inspect`四条stdin/stdout命令。

实现只复用既有Offline SDK四operation、canonical digest、strict codec、limits和context-aware streaming base64路径。它不暴露Store，不启动listener，不注册Capability，不写Owner current，不创建Effect/Settlement，不调用Application/Harness/Model/Continuity，也不包含publish/rollback/cache写/远程评测。

验证：`go test -count=100 ./sdk ./offlineapi ./cmd/context`、`go test -race -count=20 ./sdk ./offlineapi ./cmd/context`、full ordinary/race/vet均PASS。`PRAXIS_CONTEXT_MAX_SIZE=1`实测24 MiB input的request wire为33,567,978 bytes，小于48 MiB hard cap；Encode→Decode的RequestDigest与ContentSetDigest exact一致。max-size仍只作离线证据，不宣称production SLA。

边界：production Recipe发布仍为CTX-D07；production API server/root、跨Owner composition、Capability、Harness Continuation与Turn推进继续NO-GO。
