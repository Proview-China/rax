# ToolCall候选观测Projection发布与Exact Reader V1测试矩阵

## 1. 当前门

状态：reference implementation已落地，并完成第三审原子Ensure Repository与typed-nil P1修复；本矩阵仍不代表生产Backend、Continuity Adapter或composition root存在。

## 2. 单元与合同

| ID | 用例 | 预期 |
|---|---|---|
| MPR-U01 | 首次Publish合法sealed Projection | create-once成功，返回原完整Ref |
| MPR-U02 | 相同Ref与内容重复Publish | 幂等成功，Repository仍只有一份 |
| MPR-U03 | 相同Ref ID换Observation内容或digest | `Conflict`，原值不变 |
| MPR-U04 | 相同source key换Ref/内容 | `Conflict`，双索引不分叉 |
| MPR-U05 | 非法contract、revision、digest、Invocation或source | Publish在写前拒绝，零状态变化 |
| MPR-U06 | Exact Reader输入完整Ref | 返回完整Projection clone，Calls/参数无丢失 |
| MPR-U07 | Reader只给ID/CallID/ResponseID或残缺Ref | 拒绝，不执行弱查询 |
| MPR-U08 | 同ID但revision/digest/source不一致 | `Conflict`，不伪装NotFound |
| MPR-U09 | stored Projection或Observation digest被篡改 | Inspect拒绝，零部分返回 |
| MPR-U10 | 修改Inspect返回的Calls/JSON bytes | 再次Inspect结果不变，无alias |
| MPR-U11 | strict codec顶层/嵌套重复键 | Publish前拒绝，零状态变化 |
| MPR-U12 | N=1与N>1完整Projection | Reader均原样返回；调用方自行验证Calls==1，Reader不裁剪 |
| MPR-U13 | Repository线性化检查原Ref与source均从未写入 | 返回`authoritative NotFound`并携带一致性分类 |
| MPR-U14 | Retention不可读、eventual visibility或无法证明缺失 | 不得返回`authoritative NotFound` |
| MPR-U15 | 首次/重复Ensure同一sealed Projection | 在一个线性化点create或返回existing完整clone；只保存一份 |
| MPR-U16 | Ensure同ID或source换内容 | 原子`Conflict`，不部分更新任一索引 |
| MPR-U17 | zero-value内存Store调用Ensure | 安全初始化并返回exact clone |

## 3. direct顺序与原子性

| ID | 用例 | 预期 |
|---|---|---|
| MPR-W01 | sync正常终态 | 原子Ensure→exact compare→一个权威Observation event→兼容Call |
| MPR-W02 | stream正常终态 | partial零写零事件；terminal后执行同一屏障 |
| MPR-W03 | Ensure unavailable/invalid/conflict | 不重试，零权威Observation、零兼容Call、零Pending升级 |
| MPR-W04 | Ensure indeterminate | 仅以同一sealed Projection重试同一Ensure一次；其他结果零权威事件 |
| MPR-W05 | Ensure返回不同Projection或Ref | 整批Conflict，零事件 |
| MPR-W06 | N>1整批 | 保存一个完整Projection；若保留兼容事件则全部绑定同Ref并连续ordinal，仍零PendingAction/Action升级 |
| MPR-W07 | invalid/cancel/EOF/lost terminal | 零Publish或零可见Projection，零兼容Call |
| MPR-W08 | 权威事件payload与Header | 使用Ensure返回clone；ResponseID/SourceSequence/Invocation/两层digest完全一致 |
| MPR-W09 | typed-nil Repository | New/Preflight/Open/helper均在Resolve/Invoke/OpenStream前Fail Closed |
| MPR-W10 | canonical producer收到完整已sealed Projection | helper不重新seal；只Validate后跨越原子Ensure屏障并exact比较 |
| MPR-W11 | Ensure返回Validate通过但非exact sealed Projection | public producer helper、sync与stream N>1均Conflict；零权威事件、零兼容Call |

## 4. crash、lost reply与Retention

| ID | 注入点 | 预期 |
|---|---|---|
| MPR-F01 | Ensure已线性化但回包丢失 | 以同一sealed Projection重试同一Ensure一次，返回existing exact clone后继续 |
| MPR-F02 | Ensure未线性化且返回Indeterminate | 以同一sealed Projection重试同一Ensure一次，原子create后继续 |
| MPR-F03 | Ensure返回非Indeterminate异常 | 保持失败，零事件、零重试、零新Ref |
| MPR-F04 | Ensure成功、emit前模拟crash | Projection仍可exact读；不宣称全局事件exactly-once |
| MPR-F05 | split Store A Publisher/Store B Reader wrapper | 不满足原子Repository接口，不能注入Direct |
| MPR-F06 | Retention Inspection缺失/过期 | G6A InputCurrentReader Fail Closed；Projection Ref不变 |
| MPR-F07 | Projection历史可读但其他Owner current已过期 | effective current取最小上界并拒绝继续 |
| MPR-F08 | Ensure返回Projection与本地sealed Ref/source/content漂移 | `Conflict`/Fail Closed，不发事件 |
| MPR-F09 | 第二次同canonical Ensure回包再次丢失 | 停止重试并Fail Closed，不换ID、sequence或内容 |

## 5. 并发与Conformance

| ID | 用例 | 预期 |
|---|---|---|
| MPR-R01 | 32并发Publish同Ref同内容 | 全部幂等返回同Ref，只保存一份 |
| MPR-R02 | 32并发同ID不同内容 | 最多一个胜者，其余Conflict，最终值可Validate |
| MPR-R03 | 32并发同source不同ID/内容 | 最多一个胜者，source索引不分叉 |
| MPR-R04 | 64并发Inspect并修改各自返回值 | Store与其他Reader结果不变，race为零 |
| MPR-C01 | Application/Harness/Tool public imports | 只持有Reader；无Publisher、Fake、internal/vendor/Raw依赖 |
| MPR-C02 | direct注入能力 | 只接收一个具有单方法原子Ensure的Repository；缺失时Tool Observation capability不可用 |
| MPR-C03 | Continuity/host候选Adapter | 只能实现Model公共Port；测试不得命名为Production/Certified/SLA |
| MPR-C04 | Fake接口注入 | direct以原子Repository注入；只读消费者仍只持有Reader，无法类型升级获得写口 |
| MPR-C05 | 未来Continuity Adapter wire round-trip | 只映射opaque Ref/source与Content Object；读取后必须由Model strict codec重验完整Projection |
| MPR-C06 | Continuity Cursor/RecordedAt/Retention state输入current计算 | 拒绝直接type-pun成Projection TTL/current；只接受独立exact Retention Inspection |
| MPR-C07 | reference backend被标为production/RocksDB/root | Conformance拒绝；当前只有reference基础，无生产driver或composition root |
| MPR-C08 | Reader声称`authoritative NotFound` | 仅用于Reader查询分类；Direct恢复链不得读取该分类 |
| MPR-C09 | lost-reply恢复统计transport调用与Repository记录 | 同canonical Ensure最多调用两次，Repository记录恰好一份；不得声称transport exactly-once |
| MPR-C10 | Direct配置尝试把Store A Publisher与Store B Reader配对 | split wrapper无Ensure方法，不满足Repository；跨Store恢复不可表达 |
| MPR-C11 | Direct wrapper实现扫描 | 只负责seal后委托canonical producer；不得复制Ensure恢复或exact比较逻辑 |

## 6. 实施后的命令门

中央联合`YES`并授权进入代码后，至少运行：

```text
go test ./tests/toolcallobservation ./tests/executiondirect -count=100
go test -race ./tests/toolcallobservation ./tests/executiondirect -count=20
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
git diff --check
```

只有上述门禁与独立Review均通过后，才能把计划标记完成。内存Fake和Conformance通过不等于生产Backend、Retention SLA或composition root存在。
