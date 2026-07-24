# Organization Review Current V1 测试矩阵

| ID | 类型 | Oracle |
|---|---|---|
| C01 | unit | 四类 Fact stable ID 与 literal canonical digest 稳定 |
| C02 | unit | 空 tenant/id、revision 0、Checked/Expires 非法拒绝 |
| C03 | unit | roles canonical 排序、重复或未知 role 拒绝 |
| S01 | whitebox | first publish 写 history+current+highest 全有全无 |
| S02 | whitebox | same canonical publish 幂等；同 revision 换 digest Conflict |
| S03 | whitebox | expected current drift/ABA Conflict，零 history 泄漏 |
| S04 | whitebox | revision rollback/gap Conflict，旧 exact 历史仍可读 |
| S05 | blackbox | tenant A/B 同稳定 ID 互不冲突 |
| R01 | blackbox | direct Reviewer 返回 Identity+all Role+Responsibility，TTL 为真实最短 |
| R02 | blackbox | delegated Reviewer exact 绑定 Delegator/Delegate/Role/Scope |
| R03 | blackbox | veto role 只作为 Role Grant proof，不替 Review 作 veto 结论 |
| R04 | negative | production Reviewer == Responsibility Identity => Forbidden |
| R05 | negative | missing role、terminal role、role scope drift => Fail Closed |
| R06 | negative | delegation revoked/expired/cross-tenant/wrong delegate/wrong delegator => Fail Closed |
| R07 | negative | responsibility subject digest/identity drift => Fail Closed |
| R08 | negative | current index漂移但历史 fact 不变 => S2 Conflict |
| R09 | fault | S1 后 TTL crossing => zero current proof |
| R10 | fault | second clock earlier than baseline => ClockRegression |
| R11 | fault | ctx canceled/deadline => Indeterminate，不映射 NotFound |
| R12 | fault | mutation commit lost reply => only exact Inspect original ref |
| R13 | fault | Inspect lost reply under canceled original ctx => detached exact Inspect only |
| R14 | contract | Resolve unknown 没有 expected ref，不宣称恢复同一结果；只允许新 S1 |
| P01 | persistence | SQLite close/reopen exact history/current一致，WAL/integrity PASS |
| P02 | persistence | corrupted schema digest/open failure Fail Closed |
| K01 | concurrency | 64 same expected publishers only one new current revision |
| K02 | concurrency | reader与publisher并发无data race，结果只为完整旧/新 snapshot |
| D01 | import | production package不导入 Review/Harness/Application/Runtime实现 |
| D02 | boundary | public Ref不授Authority/Verdict/Evidence，module README明确root NO-GO |
