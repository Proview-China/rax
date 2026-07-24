# Review Production Service V1 测试矩阵

## 1. SQLite

| ID | 场景 | Oracle |
|---|---|---|
| SQL-01 | 首次空库启动 | schema v1创建；WAL/foreign_keys读回正确 |
| SQL-02 | 重启读取 | exact Target/Case/Trace与current index不变 |
| SQL-03 | compound create成功 | Target+Case+optional Trace同generation提交 |
| SQL-04 | Trace冲突 | generation/snapshot/history/index零变化 |
| SQL-05 | 同canonical重放 | 返回原对象；generation不无意义增长 |
| SQL-06 | same id换payload | Conflict；零写 |
| SQL-07 | 两连接同generation并发写 | 只一CAS成功；loser Conflict并可Inspect winner |
| SQL-08 | commit回包未知 | 只按idempotency/exact ref Inspect；不换payload重投 |
| SQL-09 | ctx cancel/deadline | typed Indeterminate/Unavailable；不降NotFound |
| SQL-10 | readonly/disk-full/busy | 零部分写；错误closed；恢复后原history可读 |
| SQL-11 | 损坏snapshot/digest | 启动/读取Fail Closed；不返回部分事实 |
| SQL-12 | migration失败 | writer不开放；旧文件不破坏 |
| SQL-13 | 跨tenant相同ID | 独立；不冲突、不串读 |
| SQL-14 | 历史revision exact Inspect | 不借current；旧digest仍可读 |
| SQL-15 | 纯时间推进 | 不产生新revision；ValidateCurrent按fresh now失败 |
| SQL-16 | Request携带Result Bundle | Bundle+Request+Target+Case+Trace同事务；重启exact复读 |
| SQL-17 | Bundle缺失/换digest | 五对象零写；同canonical replay返回原Bundle |
| SQL-18 | Behavior Feedback Candidate | exact Case/Target/Verdict/Policy/Reviewer/Finding闭包；lost reply只Inspect原Candidate |

## 2. Router

| ID | 场景 | Oracle |
|---|---|---|
| ROUTE-01 | restricted persistent low | Human |
| ROUTE-02 | standard low observe + explicit bypass | Bypass candidate；无Verdict |
| ROUTE-03 | permissive high reversible | Auto |
| ROUTE-04 | 任意critical | Human |
| ROUTE-05 | 任意irreversible | Human |
| ROUTE-06 | yolo但BypassAllowed=false | 不Bypass |
| ROUTE-07 | bapr+production | Fail Closed |
| ROUTE-08 | future unknown Tool ID，相同风险声明 | 与已知Tool得到相同Route；无ID allowlist |
| ROUTE-09 | Policy/Authority/Scope/Target/TTL unknown | zero Route |
| ROUTE-10 | HumanRequired=true | Human优先于Profile |

## 3. HTTP/SDK/CLI

| ID | 场景 | Oracle |
|---|---|---|
| API-01 | Submit sealed request | create-once Case；201/原ref |
| API-02 | 重复Submit同幂等/内容 | 同Case；无第二Case |
| API-03 | 同幂等换payload | 409 typed Conflict |
| API-04 | unknown/duplicate JSON key | 400；Store调用零 |
| API-05 | oversized/deep/trailing JSON | 400/413；Store调用零 |
| API-06 | tenant path/body漂移 | Forbidden/Conflict；零写 |
| API-07 | Get historical/current | 明确标记；不作为Authorization |
| API-08 | List cursor tamper/expiry | 400/412；不降级latest |
| API-09 | SSE disconnect/reconnect | Last-Event-ID恢复；允许重复，不丢已提交Trace |
| API-10 | Claim并发64 | 唯一Lease holder；Lease不授Authority |
| API-11 | approve/deny/request-changes | 只创建Attestation，不direct Verdict |
| API-12 | cancel lost reply | exact Inspect original Case；不重复mutation |
| API-13 | SDK ctx cancellation | 保留ctx错误分类 |
| API-14 | CLI输出 | JSON稳定；secret/raw webhook不输出 |
| API-15 | Finding写入 | Principal Subject必须等于current claimed Reviewer/LeaseHolder，lease过期或冒名零写 |
| API-16 | Result Bundle | Submit/SDK返回并重启复读exact Bundle；Request ref漂移零Admission |
| API-17 | Behavior Feedback | 只创建Candidate；不提供Policy/Context/Memory/Application写口 |

## 4. Platform Adapter

| ID | 场景 | Oracle |
|---|---|---|
| PLAT-01 | Slack合法签名/新timestamp | 形成Slack Observation，不形成Verdict |
| PLAT-02 | Slack重放/旧timestamp | 拒绝；零Attestation |
| PLAT-03 | Linear合法签名 | 形成Linear Observation；source nominal独立 |
| PLAT-04 | Jira合法签名/secret | 形成Jira Observation；source nominal独立 |
| PLAT-05 | 同source event ID换payload | Conflict |
| PLAT-06 | 平台identity未映射 | Unauthenticated/Forbidden；零写 |
| PLAT-07 | Envelope Target/Case漂移 | Conflict；零写 |
| PLAT-08 | external comment写approved | 仍只是Observation |
| PLAT-09 | outbound intent | 不调用网络；只返回sealed DeliveryIntent |
| PLAT-10 | raw payload超限/duplicate JSON | 拒绝；不记录secret/raw body |
| PLAT-11 | Slack payload当Linear/Jira解析 | nominal type-pun拒绝 |
| PLAT-12 | webhook处理丢回包 | source event exact Inspect；不换ID重派 |
| PLAT-13 | immutable Envelope Binding | Tenant/Envelope digest/Case/Target任一漂移在Observation前拒绝 |

## 5. 组合门禁

- targeted ordinary100：SQLite CAS/restart/router/API idempotency/platform replay；
- targeted race20：SQLite双连接、SSE订阅、Claim/Attestation并发；
- full `go test ./...`、`go test -race ./...`、`go vet ./...`；
- Store与API reusable Conformance；
- `go test -bench 'SQLite|Router|HTTP|Watch' -benchmem -count=3`只记录基线；
- `gofmt -l`、`git diff --check`、禁止import扫描；
- 无真实平台凭据时明确标记platform live E2E未执行。
