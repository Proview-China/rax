# Runtime Evidence Subject Current V1第三审NO返修待四审

- 时间：2026-07-16 23:12 CST
- 状态：第三次独立资产短审NO（P0=3/P1=4）；Runtime自有资产最小返修完成，asset-only等待第四次独立资产审计。

第三审确认旧候选把Current Index Ref/Digest放入Projection body，同时Index又绑定完整Projection Ref，形成`ProjectionDigest -> IndexDigest -> ProjectionDigest`自引用环。本次改为单向线性化：四组Owner current S1/S2闭合后先seal immutable historical Projection及完整Ref，再构造只指向该Ref的Current Index，并在同一Runtime Evidence Owner事务中原子发布Projection+Index CAS。Projection canonical不再包含任何Index字段。

首次seal固定Projection/Index稳定ID、revision=1、previous=nil和exact OwnerWatermark；后续只能同ID revision+1，Previous full-equal旧Current Projection，并以完整旧Index/Projection refs作CAS expected。Historical store不可覆盖，Current Index只单调指向新完整Ref，revision gap/rewind、same revision换digest、旧ref复活、删除重建和ABA全部Conflict。

current验证冻结四组typed current依赖：Record+Registration、Source Policy、Reader Binding+Capability+Authority、Presence+Readability。Gateway必须S1/S2 fresh双读并逐字段回扣request、Projection、Current Index和natural TTL。closed errors、first/CAS lost-reply三分、half-publish、四Owner漂移、presence/readability漂移与64并发full-ref CAS反例已同步测试矩阵。

本次仍不授权Go、Continuity adapter、production backend/root、durability或SLA；不修改旧memory、Surface Go、Evidence V2/V3或其他Owner资产。
