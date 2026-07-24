# Assembly Publication V2 测试矩阵

| ID | 场景 | 必须结果 |
|---|---|---|
| AP01 | 首次完整发布 | revision=1，四对象与current同屏障可见 |
| AP02 | Generation/Manifest/Graph/Handoff任一partial staged | Historical与Current均NotFound |
| AP03 | 四个stage写回包丢失 | 只Inspect同Publication staged digest后继续 |
| AP04 | commit回包丢失 | 只Inspect同Scope current和AttemptID恢复 |
| AP05 | 每个stage后进程崩溃并重启 | partial不可见；同内容stage幂等后可提交 |
| AP06 | 同PublicationID同staged内容 | 幂等 |
| AP07 | 同PublicationID内容漂移 | Conflict |
| AP08 | expected revision/digest过时但desired相同 | Conflict |
| AP09 | A->B后尝试A | PreviousGeneration/current不匹配，Conflict |
| AP10 | exact predecessor A->B | revision单调为2 |
| AP11 | 64并发首次发布 | 恰好1个CAS winner，其余Conflict |
| AP12 | Historical结果被调用方修改 | 后续Inspect不受影响 |
| AP13 | Current TTL跨越 | Fail Closed |
| AP14 | Current clock rollback | Fail Closed |
| AP15 | typed-nil compiler/store | 构造失败 |
| AP16 | V1 compiler/SDK既有测试 | 行为与摘要不变 |
| AP17 | full ordinary/race/vet/import/diff | 全绿且无越界写入 |
