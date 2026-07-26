# Review Internal Preview安全返修

## 事件

2026-07-26，Draft PR #2独立审查发现两项P1：本地SQLite入口未强制私有文件边界；归档验证器在结构校验前直接解包。返修只修改Review自有封包/启动/测试资产，不改变`storage/sqlite`公共语义。

## 修复

- `run-local.sh`固定`umask 077`，要求DB绝对canonical路径；
- fresh DB排他创建并验证当前用户、普通文件、单hard-link与`0600`；
- 既有宽权限DB、DB/sidecar symlink、hardlink、symlinked parent和dot path全部Fail Closed，不静默`chmod`；
- 验证器先复制不可变输入快照并核对严格单行checksum，再于任何解包前检查closed member set、唯一根、路径、重复成员、类型、mode与展开大小；
- 只按验证后的普通文件/目录清单安全解包；traversal、absolute、symlink、hardlink、device与unexpected member拒绝。

## 边界

- 同目录`.sha256`证明完整性，不单独证明发布者身份；发布来源真实性仍需可信通道摘要或后续签名；
- Internal Preview继续固定owner-local `reference_only`与`production_eligible=false`；
- 五类外部Owner、宿主root、真实Provider、OIDC、HA、备份和SLA仍production NO-GO。

## 验证状态

- `go test ./... -count=1`：PASS；
- `go test -race ./... -count=1`：PASS；
- `go vet ./...`：PASS；
- `go test ./tests ./tests/runtimeintegration -run 'Blackbox|Fault' -count=100`：PASS；
- `go test -race ./tests ./tests/runtimeintegration -run 'Blackbox|Fault' -count=20`：PASS；
- `scripts/test-internal-preview.sh`：确定性双构建、真实SQLite+HTTP+CLI、DB权限/symlink与relative-traversal/absolute-path/symlink/hardlink/unexpected-member恶意归档门禁全部PASS；
- `gofmt -l`、`go mod tidy -diff`、shell syntax/import/diff检查：PASS。
