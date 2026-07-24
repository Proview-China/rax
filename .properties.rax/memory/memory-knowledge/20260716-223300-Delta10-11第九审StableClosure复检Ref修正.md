# Delta 10/11第九审StableClosure复检Ref修正

时间：2026-07-16 22:33 CST

## 问题

冻结草案的Memory/Knowledge StableClosure canonical body曾包含`AttemptInspectionRef`，但`AttemptInspectionV2` canonical包含fresh `OwnerCheckedAt/ExpiresAt`。重新Inspect可能产生不同Inspection ref，导致TTL或复检时间变化错误污染stable closure。

## 修正

- 从两OwnerStableClosure canonical body删除`AttemptInspectionRef`。
- fresh `CurrentProjectionV2`继续保留`AttemptInspectionRef`，因此重新Inspect会改变fresh Projection digest。
- S2 `CurrentRequestV2`不携带S1 Inspection ref；S2只比较Source coordinate、StableClosureDigest与ordered exact集合，并使用fresh Owner clock/TTL裁决。
- 增加N72与Conformance 25，固定“Inspection ref变化、stable closure不变、fresh digest变化”的TTL/复检反例。

## 当前真值

该修正关闭第九审Owner P1候选后，Owner-local恢复P0=0/P1=0/P2=0；External P0=5保持NO-GO。live V1仍是唯一实现真值，本轮未写Go。
