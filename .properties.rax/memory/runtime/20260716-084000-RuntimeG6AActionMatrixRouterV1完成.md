# Runtime G6A Action Matrix/Router V1完成

## 事件

2026-07-16，Runtime完成G6A Action Matrix/Router V1最小实现：唯一Run内Tool Action矩阵、五维Owner-current封闭路由、Runtime-neutral Provider Boundary只读合同、受控test Provider seam与public-only Conformance进入验证收口。

## 已闭合语义

- 唯一矩阵为`run + praxis.tool/execute + praxis.tool/single-call-action-v1`，Generation与Run/Session/Turn/Action/Context全部required；
- Owner source仅按`Kind/ID/Revision/Digest`无损投影，Runtime不创建Applicability Fact/Store；
- Router拒绝缺失、重复、未知Kind、Owner version不匹配、current projection漂移与TTL过期；
- Provider调用前必须依次获得exact/current execute Enforcement 4.1、execute Evidence Handoff和Tool Owner Boundary current proof，并逐字段绑定同一Operation/Scope/Attempt；
- 64并发同Boundary只形成一个逻辑fixture调用；Reader unavailable、漂移、过期、type-pun和lost reply恢复均不盲目重派；
- Runtime不写Tool Watermark，不预填Evidence Consumption，不创建DomainResult或Operation Settlement。

## 保留边界

该实现仅供隔离test fixture与Conformance，不是production composition root，不声明真实Provider backend、持久性、availability、SLA或物理exactly-once；不启用Capability、Context Refresh、Continuation、Turn推进或N>1。
