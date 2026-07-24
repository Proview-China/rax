# 官方Prompt Provenance离线核完成

时间：2026-07-18 01:10（Asia/Shanghai）

用户确认不同Model的预埋提示词优先借鉴厂商官方开源Coding Agent实现，并要求兼容Codex、Gemini、Kimi、MiniMax、Claude SDK、DeepSeek、Grok及同步演进的T3Code。Context Owner完成官方来源分级：Codex/Gemini/Kimi/Grok为可审计Coding Agent明文候选；Claude只保留官方SDK `claude_code` preset引用；DeepSeek/MiniMax只作为模型template/profile证据；OpenCode只作B级差异Evidence。T3Code仅为消费端，不拥有Prompt/current/Actual Injection。

新增`PromptUpstreamProvenanceV1`与纯离线Verify核，exact seal repo/40-hex commit/path、artifact与license bytes、byte range、license review evidence、transform step连续性、Generated ContentRef集合及Stable/SemiStable/Dynamic closure。首版总输入hard max 32MiB，hash/clone按64KiB检查context；所有输出deep-copy/no-alias。Opaque SDK preset允许零GeneratedContent，但其他来源不得借此绕过正文闭包。Verify成功只产生Context verification report，不授予Authority、Review、published、Model适用性或ActualInjection。

实现文件：`contract/prompt_provenance.go`、`kernel/prompt_provenance.go`、`internal/testkit/prompt_provenance.go`及contract/kernel/blackbox/failure/conformance tests。实际验证：定向普通100轮PASS、race20轮PASS、full `go test ./...` PASS、full `go test -race ./...` PASS、`go vet ./...` PASS。

production仍NO-GO：Context不定义第二套ModelFamily/Route/Profile nominal。Model Invoker live只有可计算digest的`SemanticRouteProfile`而无公共nominal exact Profile ref/current reader；该Port Delta闭合前不能按route current选择Prompt，也不实现T3Code/Application/Harness production Adapter、联网抓取或Prompt发布。
