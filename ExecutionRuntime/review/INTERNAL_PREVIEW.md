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

启动器固定使用私有`umask 077`。`PRAXIS_REVIEW_DB`必须是绝对canonical路径，所有既有父目录都必须是真实目录且不能是symlink：

- fresh DB由启动器排他创建并验证为当前用户所有、普通文件、单一hard-link、`0600`；
- existing DB及既有`-wal`、`-shm`、`-journal`同样必须已经满足上述私有边界；
- 过宽权限、非当前用户、symlink、hardlink、symlinked parent或dot path component一律Fail Closed；
- 启动器不会用`chmod`静默“修复”不安全的既有数据库。

### 启动前诊断

使用同一启动配置执行check-only模式：

```bash
PRAXIS_REVIEW_MODE=check ./bin/run-local.sh
```

该模式严格执行配置/Secret shape、监听地址/TLS约束、SQLite open/migration/WAL/foreign-key及`PRAGMA integrity_check`，但不创建HTTP listener。fresh DB会初始化schema，因此它不是纯只读命令；既有损坏数据库、未知mode、坏TLS或非loopback无TLS均以非零状态失败。

成功只输出不含路径、token或其他Secret的稳定JSON：

```json
{"contract_version":"praxis.review.service-check/v1","status":"ok","configuration":"valid","database":"ready","integrity":"ok","listener":"not_started","support_mode":"owner-local","production_eligible":false}
```

这里的`database=ready`只表示Review-owned单节点启动依赖已通过，不表示跨Owner current、Runtime Gate、Provider或Praxis production readiness。

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
2. 在任何解包前验证唯一根目录、closed member set、路径、类型、mode和展开大小；
3. 拒绝绝对/`.`/`..`路径、重复或unexpected member、symlink、hardlink、device等非预期类型，再按已验证清单安全解包；
4. 检查manifest边界；
5. 启动真实`review-service`二进制和临时SQLite；
6. 验证未认证请求为401；
7. 使用真实`praxis-review` CLI经HTTP列出空Case集合；
8. 发送SIGTERM并确认服务正常退出。

同目录`.sha256`用于完整性和重复构建校验，不单独证明发布者身份。需要来源真实性时，调用方仍须从可信发布通道取得并核对摘要或后续签名；替换archive与checksum也无法绕过验证器的预解包结构门禁。

完整重复构建、黑盒与故障门禁：

```bash
./scripts/test-internal-preview.sh
```

该门禁执行两次独立封包并比较归档字节，随后执行check-only启动诊断、真实服务/CLI冒烟、损坏DB/未知mode、篡改归档、非法Secret、DB权限/symlink路径，以及relative-traversal/absolute-path/symlink/hardlink/unexpected-member恶意归档拒绝。

## 明确不包含

- Binding、Evidence、Policy、Authority、Scope等外部Owner的production Adapter/certification；
- Application/Harness/Model/Context/Continuity的宿主composition root；
- Runtime Authorization、Permit、Begin或Effect Dispatch；
- 真实Auto模型调用、Slack/Linear/Jira outbound Provider与Webhook identity admission；
- OIDC、HA、备份、跨节点一致性、容量或SLA承诺。

该封包中的Review事实不能作为现实动作已经获得执行授权的证明。
