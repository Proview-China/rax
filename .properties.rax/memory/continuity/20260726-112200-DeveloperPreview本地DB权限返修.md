# Developer Preview本地DB权限返修

## 返修结论

`cmd/continuity-reference`不再让SQLite依赖调用方`umask`创建metadata DB。CLI入口在进入`storage/sqlite.Open`前执行本地路径安全前置：

- fresh DB通过exclusive create显式使用`0600`；
- existing DB必须是当前有效用户拥有的regular file，权限精确为`0600`；
- existing权限过宽时拒绝，不静默`chmod`；
- DB文件或任一父路径为symlink时拒绝；
- 缺失父目录、非目录父路径，以及未受sticky/private祖先保护的group/other-writable路径拒绝。

SQLite公共SPI、schema与持久化语义未修改。该检查用于降低同机其他用户读取或替换metadata的风险，不声明跨进程目录租约、descriptor-relative no-follow或production secret store能力。

## 固定反例

- `umask 022`下fresh DB最终mode必须为`0600`；
- existing `0644`必须Fail Closed且不得被静默改权；
- DB symlink与symlink parent必须Fail Closed；
- directory-as-DB与missing parent必须Fail Closed；
- 所有拒绝路径不得产生部分stdout；
- 治理写命令仍因Application Gateway缺失而Fail Closed。

## 保持边界

本返修不增加Timeline写、Checkpoint capture、Restore Execute、remote Provider、Praxis root CLI、跨Owner Participant、production deployment或SLA。

## 实际验证

- 权限/path与既有read-only/写Fail Closed定向测试：`count=100` PASS；
- 同一矩阵：`race count=20` PASS；
- `go test -count=1 ./...`、`go test -race -count=1 ./...`、`go vet ./...`：PASS；
- RocksDB build-tag ordinary/race/vet（`./storage/rocksdb ./tests/blackbox`）：PASS；
- `umask 022`真实运行：fresh DB=`0600`；
- existing `0644`真实运行：exit 1、mode仍`0644`、stdout 0字节；
- DB symlink真实运行：exit 1、stdout 0字节。
