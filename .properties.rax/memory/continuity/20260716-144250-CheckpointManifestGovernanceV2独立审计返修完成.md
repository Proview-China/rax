# Checkpoint Manifest Governance V2独立审计返修完成

时间：2026-07-16 14:42:50 CST

状态：Continuity独立代码审计P1/P2返修完成；C-02 Continuity Owner切面可交同一独立审计复验。Checkpoint跨Owner接线与Restore运行面继续`NO-GO`，Provider调用为`0`。

## 本次返修

- `ExactFactRefV2`新增Tenant绑定；Manifest递归校验Context、Memory、Knowledge、Attempt、Settlement/Inspection、Participant、Snapshot/Coverage、Evidence、Diagnostic与Residual全部exact refs和Manifest `TenantID + ExecutionScopeDigest`一致，拒绝跨Tenant/Scope splice；
- 聚合Manifest顶层、所有Attempt（包括已有Settlement）、所有Participant及任意severity Diagnostic的Residual；`verified_candidate`与Seal要求聚合Residual为空；
- Seal Repository在单一事务内复读current Manifest，重验exact revision、`verified_candidate`、Owner及Manifest/Attempt/Barrier/EffectCut/frozen/required/Participant全部binding；直接Repository调用不能绕Controller；
- Manifest current/history及Seal key改为结构化`TenantID + ScopeDigest + ID`；current Reader请求携受验Scope与Continuity Owner Binding，同ID可在不同Tenant/Scope独立存在且不可串读；
- CAS lost-reply仅在current恰为expected+1且与next exact一致时幂等；current继续前进后重放旧CAS返回Conflict，immutable history不变且不形成ABA；
- 新增递归splice、嵌套Residual、直接Repository、跨Tenant同ID、64路不同内容CAS/Seal单赢家、progressed lost-reply/no-ABA反例；保留typed-nil、clone/no-alias、canonical tamper、Unknown只Inspect、Checkpoint/Restore执行NO-GO反例。

## 实际验证

工作目录：`ExecutionRuntime/continuity`

```bash
go test ./contract ./domain ./storage/memory ./tests/fault ./tests/conformance
go test -count=1 ./...
go test -count=1 -shuffle=on ./...
go test -count=100 ./contract ./domain ./fakes ./storage/memory ./tests/blackbox ./tests/fault ./tests/conformance
go test -race -count=20 ./domain ./fakes ./storage/memory ./tests/fault ./tests/conformance
go test -race -count=1 ./...
go vet ./...
gofmt -l .
go list -deps ./...
rg -n '"github\.com/Proview-China/rax/ExecutionRuntime/(runtime|harness|application|sandbox|context-engine|tool-mcp|model-invoker|memory-knowledge|review)/' --glob '*.go' .
```

结果：定向包PASS；full ordinary/shuffle PASS；ordinary `count=100` PASS；定向race `count=20` PASS；full race PASS；vet PASS；`gofmt -l`无输出；`go list -deps`只有标准库与本模块包；禁止跨Owner实现import扫描无命中。

资产轻门：Continuity Markdown相对链接、围栏平衡、draw.io XML、陈旧状态词、尾随空白和限定范围`git diff --check`均PASS。暂存区文件数为`0`，未stage、未commit。

## 保留边界

- Continuity只拥有Manifest/Seal Fact、Inspect/CAS/diagnostic/residual；不拥有Runtime Attempt/Barrier/EffectCut/Consistency、Harness/Participant Snapshot或其他Owner Fact；
- Partial/Indeterminate/Rejected只作诊断，不生成Consistency；Unknown/lost reply只Inspect原identity；
- Restore仍只保留reference-only Plan/shape验证，不Stage、不Activate、不创建Instance/Lease/Fence，不宣称外部世界回滚；
- legacy Checkpoint/Restore接口不得补默认治理字段或扩权；SQLite/RocksDB仍只有SPI，production driver、remote blob/purge/archive、SDK/CLI/API与生产SLA继续NO-GO。
