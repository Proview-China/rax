# Delta 10/11第九审Retrieval Envelope摘要补齐

时间：2026-07-16 22:37 CST

## 问题

两Owner冻结草案的`CurrentProjectionV2`与StableClosure canonical body只携带Coverage与Items，遗漏live `RetrievalResult`及上层设计要求的`NextCursor`、`ResultDigest`、`EvidenceDigest`，导致分页边界或结果/evidence摘要变化可能不改变canonical。

## 修正

- Memory与Knowledge `CurrentProjectionV2`均按`Coverage -> NextCursor string -> ResultDigest string -> EvidenceDigest string -> Items`加入字段。
- 两OwnerStableClosure canonical body使用完全相同的字段类型与顺序。
- 三字段全部进入fresh Projection digest与StableClosureDigest；任一变化必须改变两个canonical摘要。
- N70/N71补字段完整性要求，新增N73与Conformance 26固定NextCursor/Result/Evidence变化反例。

## 当前真值

该修正关闭第九审新增Owner P1后，Owner-local为P0=0/P1=0/P2=0；External P0=5保持NO-GO。live V1仍是唯一实现真值，本轮未写Go。
