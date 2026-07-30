# Context Frame Exact Current Durable Reader实现候选

## 结果

Context Owner在独立`agent/context-frame-current-reader-v1`工作树形成
`ContextFrameExactCurrentReaderV1`的单节点SQLite durable实现，替换此前
model-input lineage只能注入测试fixture的Frame current缺口。当前为Owner自验通过的
Review候选。独立终审曾以`P0=0/P1=2`退回schema物理闭包；两项P1已返修并完成
全套Owner复验，当前等待再次独立审计，不在复审前标记最终完成。

## 已落地

- `framestore.SQLiteV1`使用WAL、`synchronous=FULL`、STRICT schema；
- Frame/Manifest/Generation完整closure写入append-only history，current pointer以
  expected Generation current CAS原子推进；
- Frame public exact坐标在同一Owner内跨scope全局唯一，ID只作查询键，revision与
  digest必须exact；
- operation ledger完整绑定Owner、scope、run、session、predecessor、next pointer、
  Frame与`state_row_digest`，lost reply只Inspect原operation；
- Frame exact-current与Generation current pointer读取都在两个独立只读事务中核对
  完整current row、history maximum、Frame/Manifest/Generation payload exact refs，
  使用fresh Owner clock执行S1/S2和TTL最小值；第二SQLite连接在S1/S2窗口合法advance
  时返回Conflict与零projection/pointer；
- model-input lineage构造器要求Frame Reader公开并匹配Context Owner binding，拒绝
  cross-Owner reader splice；
- schema verifier用`table_xinfo`逐表核对STRICT列、cid/type/NOT NULL/default/PK/
  hidden，拒绝generated列；对Context Frame store所属table/index/trigger及SQLite
  自动PK index建立精确对象闭集；引号感知、去注释的canonical DDL比较闭合完整
  CREATE TABLE/INDEX/TRIGGER语义，trigger引号内注释文本不会被错误剥离；
  `index_xinfo`严格闭合连续seq、key/aux cid/name/desc、字面BINARY collation与升序；
  所有`table_xinfo`、`index_list`、`index_xinfo`、`foreign_key_list`迭代都检查
  `Rows.Err`与`Close`错误；完整FK字段/顺序/action、trigger表/event/body均exact；
- Open时在同一事务执行合法history+current+ledger全链正向probe并强制rollback，
  再以随机rollback probe证明业务CHECK与append-only行为，probe不得留下可见状态。

## 反例与验证

专属测试覆盖exact commit/read、advance后旧Frame失效、完整predecessor重启恢复、
operation同ID异payload Conflict、lost reply Inspect、S1/S2中间漂移、
now==expiry、clock rollback、future-created、commit跨TTL、64并发单current、
第二SQLite连接跨S1/S2 advance、
Owner/metadata/current/ledger splice、noncanonical receipt，以及comment/weak CHECK、
no-op/错表trigger、错列/错序/错action FK、wrong-table/NOCASE exact index、
generated列与额外table/index/trigger。新增反例覆盖额外table CHECK、非索引列
`COLLATE NOCASE`、trigger引号内`/*not a comment*/`、跨scope复用相同public
Frame ID/revision，以及合法正向probe零残留。

实际执行：

- focused schema/public-coordinate回归`count=20`：PASS；
- framestore/blackbox/conformance/failure定向`count=100`：PASS；
- 同组`-race -count=20`：PASS；
- `go test ./...`：PASS；
- `go test -race ./...`：PASS；
- `go vet ./...`、`gofmt`、`git diff --check`：PASS。

## 边界

本切片是Context-owned单节点durable Store与authoritative Reader，不修改既有Material/
Frame V1摘要语义，不调用Runtime Settlement、Provider、Harness或Model，不注册
Capability。Application/Harness/Model production composition root、Continuation、
Turn推进、多节点HA、备份与SLA仍为NO-GO；测试fixture不属于production backend。
