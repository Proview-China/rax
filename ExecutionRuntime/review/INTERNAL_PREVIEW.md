# Review Engine 单节点 Internal Preview

该封包用于在单机上运行 Review-owned 的 SQLite、HTTP、Go SDK 与 CLI 软件面。它可以管理 Review Request、Case、Assignment、Attestation、Finding、Trace 与 Result Bundle，但不包含 Praxis 宿主的公共 Review Gate、Runtime Authorization、真实 Auto Reviewer Provider 或外部通知执行点。

## 封包内容

```text
praxis-review-internal-preview/
|-- bin/
|   |-- praxis-review
|   |-- review-service
|   `-- run-local.sh
|-- docs/
|   |-- INTERNAL_PREVIEW.md
|   `-- README.md
`-- manifest.json
```

`manifest.json` 固定声明：

- `channel=internal-preview`；
- `support_mode=owner-local`；
- `production_eligible=false`；
- 构建所用源码revision、Go版本、目标平台与源码时间戳。

## 构建

从`ExecutionRuntime/review`运行：

```bash
./scripts/package-internal-preview.sh
```

默认产物位于`dist/`。构建要求：

- Go 1.25；
- GNU `tar`、`gzip`与`sha256sum`；
- 干净的Review模块工作区。

脚本使用`CGO_ENABLED=0`、`-trimpath`、`-buildvcs=false`和固定tar时间/owner生成确定性封包。同一源码revision、Go工具链、GOOS与GOARCH应产生相同SHA-256。

开发中的未提交改动只允许显式执行：

```bash
PRAXIS_REVIEW_ALLOW_DIRTY=1 ./scripts/package-internal-preview.sh
```

该模式会在manifest中标记`source_dirty=true`，不得作为可复现发布证据。

## 本地启动

解压后生成两段随机hex Secret：

```bash
export PRAXIS_REVIEW_TOKEN="$(openssl rand -hex 32)"
export PRAXIS_REVIEW_CURSOR_KEY_HEX="$(openssl rand -hex 32)"
export PRAXIS_REVIEW_DB="$PWD/review.db"
export PRAXIS_REVIEW_ADDR="127.0.0.1:8087"
export PRAXIS_REVIEW_TENANT="local-preview"
./bin/run-local.sh
```

`run-local.sh`不会生成、保存或打印Secret。它只接受至少64个hex字符的Bearer token和cursor key，并在进程内构造当前单租户静态鉴权配置。非loopback监听仍必须显式设置`PRAXIS_REVIEW_TLS_CERT`与`PRAXIS_REVIEW_TLS_KEY`。

另一个终端可使用CLI：

```bash
export PRAXIS_REVIEW_URL="http://127.0.0.1:8087"
export PRAXIS_REVIEW_TOKEN="<与服务相同的token>"
./bin/praxis-review list --tenant local-preview
```

## 验证封包

```bash
./scripts/verify-internal-preview.sh \
  ./dist/praxis-review-internal-preview_linux_amd64.tar.gz
```

验证器会：

1. 校验归档SHA-256；
2. 解包并检查manifest边界；
3. 启动真实`review-service`二进制和临时SQLite；
4. 验证未认证请求为401；
5. 使用真实`praxis-review` CLI经HTTP列出空Case集合；
6. 发送SIGTERM并确认服务正常退出。

完整重复构建、黑盒与故障门禁：

```bash
./scripts/test-internal-preview.sh
```

该门禁执行两次独立封包并比较归档字节，随后执行真实服务/CLI冒烟、篡改归档拒绝和非法Secret拒绝。

## 明确不包含

- Binding、Evidence、Policy、Authority、Scope等外部Owner的production Adapter/certification；
- Application/Harness/Model/Context/Continuity的宿主composition root；
- Runtime Authorization、Permit、Begin或Effect Dispatch；
- 真实Auto模型调用、Slack/Linear/Jira outbound Provider与Webhook identity admission；
- OIDC、HA、备份、跨节点一致性、容量或SLA承诺。

该封包中的Review事实不能作为现实动作已经获得执行授权的证明。
