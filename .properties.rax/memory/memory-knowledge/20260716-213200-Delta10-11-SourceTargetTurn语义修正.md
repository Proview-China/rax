# Delta 10/11 Source/Target Turn语义修正

时间：2026-07-16 21:32 CST

## 当前真值

- Memory/Knowledge Owner-local Current Reader V1已完成并通过第三次独立复审YES；V1及各自Owner Store/Journal/current仍是唯一live真值。
- Delta 10/11 V2只是同一Reader合同族的`design_candidate / review_pending`加法候选；不存在第二Owner DTO、public facade、Store或current。
- 未修改Go；Context/Application Adapter、production root、远程Retrieval Gateway与非零G6B来源继续NO-GO。

## 本次修正

1. Source/ActionTurn=T必须exact等于settled Tool `Execution.Turn`与`ExpectedCurrent.Turn`。
2. Memory/Knowledge V2 Reader只验证并回显各自retrieval-bound SourceTurn；V1 `TurnID`只映射为`SourceTurn.ID`。
3. Target/ContextTurn=T+1属于Context childExecution、新Frame与新Generation，只能由Context/Application唯一transition proof无损关联。
4. Reader不得携带TargetTurn、执行`Ordinal+1`或构造transition proof；两个Owner不得补Session，Application不得按Ordinal自造Target ID/Revision/Digest。
5. Context必须逐项验证两个Owner SourceTurn与独立transition proof；跨Owner T/T+1混用、proof缺childExecution/Frame/Generation或SourceTurn不等于Tool Execution/ExpectedCurrent均Fail Closed。

## 待联合Owner冻结

- P0：Memory/Knowledge V2的Identity exact ref/epoch、retrieval-bound Session与SourceTurn exact字段。
- P0：Context/Application transition proof的version、canonical字段、expiry及Source=T→Target=T+1、childExecution/new Frame/Generation绑定。
- P0：Context `knowledge_reference` exact source binding、公共Refresh Port、原子Apply+Generation CAS与跨模块fixture。
- P1：V2预算/Estimator、Association/Source/Citation set digest及原error set必要加法。
- P2：diagnostics、Citation/Residual展示字段。
