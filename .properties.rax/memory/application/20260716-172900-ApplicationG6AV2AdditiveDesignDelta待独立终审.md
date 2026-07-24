# Application G6A V2 additive设计Delta进入独立终审

时间：2026-07-16 17:29（Asia/Shanghai）

## 事件

Application `SingleCallToolAction V2`四份资产已按live公共合同形成additive候选，状态从旧的“联合设计终审YES”纠正为“联合候选 / 待独立终审”。本事件只记录设计变化，没有写Go、stage或commit。

## 已闭合的候选边界

- 版本闭包固定为Harness BindingV2、Session/CAS V4、Subject/Request/Current/Reader V3；ReaderV2禁止承载新类型；
- Application neutral Binding覆盖Base四事实、OwnerInputs五项、Harness BindingVersion/Digest；
- Owner current证明缩为`HarnessOwnerCurrentProofV3 + AuthorityCurrentProofV2`；
- `SingleCallActionCoordinateV2`先于Authority形成，Runtime Authority exact绑定其digest；
- Request TTL删除无输入的Policy；
- ResultCoordinate/Result/ResultRef/InspectKey/Tool Port/只读Settlement Reader/Coordination Fact/CAS V2的字段、canonical domain、状态机和lost-reply恢复已形成候选；
- Identity补`CreatedUnixNano`并进入canonical；
- Route V2第八轮审计YES真值已同步，V1仍只算owner-local。

## 当前门禁

Application Go实施继续冻结。必须先完成本design/plan独立终审；即使后续owner-local实现完成，系统G6A、production composition root、G6B、Continuation、Turn推进、Capability、Checkpoint与N>1仍分别NO-GO。
