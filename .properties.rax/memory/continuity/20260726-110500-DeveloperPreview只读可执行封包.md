# Developer Preview只读可执行封包

## 粗粒度进展

Continuity现有SQLite metadata store、transport-neutral SDK和CLI Runner已经装配成可直接构建运行的`cmd/continuity-reference`。入口只允许：

- `timeline show`
- `timeline watch`
- `checkpoint inspect`

CLI Runner新增显式只读构造器。缺少Application治理Gateway时，`timeline project`、`checkpoint create`、`fork`、`rewind plan`、`restore`、`artifact attach`、`retention resolve`和`workflow inspect`全部Fail Closed，不能绕过公共治理链。

## 边界

- 这是`reference_only` Developer Preview，不是`praxis`根CLI、独立服务、SDK production root或Provider。
- 打开数据库会创建或迁移本地SQLite metadata schema；不写Timeline Event、不创建Checkpoint、不捕获Snapshot、不执行Restore/Rewind、不调用remote Provider。
- SQLite/RocksDB、Manifest/Seal、Restore/Rewind Plan和Release Candidate既有边界不变。
- production trusted Assembler、真实typed Owner Readers、跨Owner全量Participant、credential/deployment attestation、remote blob/purge/archive及SLA仍未闭合。

## 验证范围

本切片增加：

- 只读Runner执行Timeline读取；
- 治理写命令无Gateway时零输出并Fail Closed；
- 可执行入口创建/打开SQLite并返回strict JSON Timeline page；
- 缺数据库参数或命令时返回usage；
- ordinary、race、vet与RocksDB可选构建回归。

## 实际结果

- `go test -count=1 ./...`：PASS
- `go test -race -count=1 ./...`：PASS
- `go vet ./...`：PASS
- `go test -count=1 -tags continuity_rocksdb ./storage/rocksdb ./tests/blackbox`：PASS
- `go test -race -count=1 -tags continuity_rocksdb ./storage/rocksdb ./tests/blackbox`：PASS
- `go vet -tags continuity_rocksdb ./storage/rocksdb ./tests/blackbox`：PASS
- 新增只读Runner/可执行入口定向`count=100`与`race count=20`：PASS
- 实际构建运行：空SQLite执行`timeline watch`返回strict JSON与有效Cursor；`restore`退出1、stdout为0字节且明确拒绝缺失的governed workflow capability。
