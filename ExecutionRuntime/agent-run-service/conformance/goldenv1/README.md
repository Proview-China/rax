# Cross-language Wire Golden V1

本目录从 `contract` 中已经 seal 的公共类型生成稳定 JSON。它用于 Go 与未来 generated TypeScript client 的 golden/conformance，不是另一份 IDL，也不提供 Console/Page DTO。

覆盖：

- decimal string 的 uint64、int64、UnixNano 与 exact ref/digest；
- AgentRunService 六个方法的代表性 request/result；
- optional absent 与 explicit null 拒绝；
- unknown enum fail closed；
- 全公共 Fault 映射；
- command idempotency replay/conflict；
- cursor resume 与 retention gap `RESYNC_REQUIRED`。

生成结果必须与 `testdata/wire-v1` byte-for-byte 相等：

    go run ./conformance/goldenv1/cmd/generate ./testdata/wire-v1
    git diff --exit-code -- ./testdata/wire-v1
