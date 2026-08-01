# Workspace Read Command Publication V2 Module

状态：`implementation-software-test-yes / owner-local / uncommitted`。

本模块计划把一个external Owner的exact workspace-read Source command current，
经Runtime Effect/Prepared与Sandbox Workspace current双读闭包，编译为：

```text
WorkspaceReadCommandV1
+ WorkspaceReadCommandPublicationV2
+ WorkspaceReadCommandOwnerCurrentV2
```

公开面只包含Sandbox-neutral Source current消费Reader、coordinate-only Ensure、
Publication exact Reader与OwnerCurrent exact/current Reader。它不接受generic
ObjectRef、caller snapshot、stable key或raw Owner writer。

neutral Source exact coordinate显式封存role=`settlement`的`EffectOwnerRefV2`、固定Kind
`praxis.tool/workspace-read-execution-command`及ID/revision/digest；owner/manifest
必须存在于Effect.Intent.Owners exact集合，并与Runtime Effect/Prepared Provider
闭合。Source Ref只接受raw lowercase
64 hex，payload digest只接受`sha256:` lowercase canonical表示。

计划中的唯一writer由Sandbox Go internal capability保护；public interface和
concrete SQLite Store均不保留raw Command create。在SQLite v19初次单事务写入
Command canonical/body seal、immutable Publication和OwnerCurrent；refresh仅对
同一Publication CAS追加短期OwnerCurrent。
schema ledger只由Open/migration维护，Ensure事实事务绝不执行DDL或推进ledger。

Command/Publication使用stable absolute semantic NotAfter，不能固化每次重签的
outer current digest/expiry；transient proof只进入OwnerCurrent。v18事实只保留
historical exact读取，不回填、不续期，也不再仅凭TTL获得V1 current/physical或
V2资格。

上游发行水位只提供`EffectiveCreatedLower`。Sandbox Command/Publication winner
记录真实owner commitNow，但该Created不参与stable Ref/digest/equality。

当前已落下public nominal contract、窄ports、kernel owner factory、internal
capability、owner-current initial/next seal、stored三事实与fresh四Reader join、
SQLite v19三事实/body-seal/history/pointer、8-handle create/refresh、restart与
lost-reply门禁。public raw Command writer已移除；physical executor与completion
都只能消费Publication+OwnerCurrent fresh资格，V1/raw-only Store不能再组成执行
旁路。ordinary、race、vet、schema/import/no-bypass与diff门禁已通过。

这里的factory ready仅指Sandbox owner-local Command Publication闭包。Tool neutral
adapter、cross-owner production composition、Provider、CorePack Executable与
production root仍继续NO-GO；本模块不生成Console合同。
