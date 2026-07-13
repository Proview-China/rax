# Codex Pro临时登录与App Server真实验证完成

- 时间：2026-07-13 23:59（Asia/Shanghai）
- 模块：`ExecutionRuntime/model-invoker`
- 状态：Codex Pro官方CLI与Praxis App Server真实验证通过；临时认证在本轮收口时删除

## 真实结果

使用独立临时`CODEX_HOME`发起官方device login，由用户手动完成ChatGPT登录。现场只读取安全声明确认计划类型为Pro，不打印token、账号标识或原始认证内容。临时`auth.json`权限为0600，未复用、覆盖或修改用户`~/.codex/auth.json`。

官方`codex exec`以`gpt-5.6-sol`调用`chatgpt.com/backend-api/codex/responses`并返回精确marker，已确认订阅额度能够正常使用。随后Praxis生产Codex Adapter在只读、never approval、ephemeral thread、禁用工具的条件下完成真实`Preflight → initialize → thread/start → turn/start → stream → terminal`，返回精确marker且无副作用。

## 真实联调发现并修复

1. 当前Codex App Server协议省略`jsonrpc: "2.0"`。新增`codex_app_server_ndjson`协议方言；ACP继续严格要求JSON-RPC 2.0，未扩大协议接受面。
2. Codex订阅可用模型现场为`gpt-5.6-sol`。代表Profile从旧`gpt-5.6`更新，并刷新模型与协议revision。
3. fake App Server原先错误携带`jsonrpc: "2.0"`，掩盖真实不兼容。fake、协议单测和离线集成已改为versionless，并新增“Codex接受省略版本、严格JSON-RPC拒绝省略版本、Codex拒绝显式错误版本”的反例。
4. Harness清洗环境最初删除代理变量，直连命中Cloudflare限制；显式代理白名单加入live smoke，值不进入可读Manifest，只记录变量名和整体摘要。
5. 当前代理不支持Responses WebSocket。旧feature flag不能覆盖模型目录的WebSocket偏好；通过官方Codex自定义HTTP-only provider选择HTTPS/SSE，仍由官方Codex使用ChatGPT认证并直达官方后端，不经过Praxis反代。
6. Codex原生`error`包含`codexErrorInfo`和`willRetry`。可重试流错误现在继续等待官方重试，不再立即伪造失败终态；不可重试错误只输出安全枚举，不打印原始message。
7. 连接smoke原先同时发送`outputSchema`，把能力验证与登录/连通性耦合。现拆为无工具marker握手并在Praxis本地严格解析；结构化输出保留为独立conformance验证项。

## 验证

- 官方Codex CLI真实订阅marker：通过；已知一次调用报告2702 tokens，HTTP-only provider独立验证报告6382 tokens；
- Praxis Codex App Server真实单Route marker：通过；
- `./scripts/verify-offline.sh`：通过，包含gofmt、module verify、tidy diff、vet、普通、shuffle、race、完整integration-tag离线套件与catalog assets；
- 凭据与原始native error未进入测试输出或仓库资产。

## 后续边界

Codex严格`outputSchema`、Tool Call、文件操作、审批和取消仍应作为独立真实能力用例逐项验证，不能由本次最小登录/连通smoke外推。Claude、Gemini、Kimi、Qwen及直接API Route仍等待对应真实账号或Key。
