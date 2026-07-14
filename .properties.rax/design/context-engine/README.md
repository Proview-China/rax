# Context Engine、Cache Manager与Runtime交接

## 1. 状态

- 当前阶段：Context/Cache的Runtime-facing安全合同已修正；组合与缓存算法仍待Context模块单独设计；
- 实现授权：无。

## 2. 作用与组成

Context Engine构建有来源、不可变、可审计的ContextPackage，不擅自改写用户或Prompt资产语义。

```text
Context Composer
Prompt Asset Resolver
Token Budget Manager
Context Manifest Builder
Cache Manager
  - Capability Resolver
  - Stable Prefix / Key / Affinity Planner
  - TTL与失效
  - Local Cache
  - Provider Cache Plan
  - Usage与节省证据
```

## 3. Cache安全

Cache Partition至少绑定Tenant、Principal/Authority、数据分级、Context与来源摘要、Prompt资产、Provider/Deployment/Model、Harness、Route、Tool Schema和Provider Cache状态。命中时重新鉴权。V1禁止跨租户、跨权限和跨Harness隐式复用。

Provider Cache创建/写入/删除/Retention变化或独立远程查询是数据披露、费用或持久Effect，必须有Intent、Fence、Retention和Receipt。模型请求内部的Provider Cache读取由父模型Intent覆盖；本地无外发无费用查找可不创建独立Runtime Intent，但必须重验Partition/Authority并记录CacheAccessEvidence。任何命中在内容返回前重新鉴权。无法证明Provider隐式缓存隔离时，敏感Context禁止使用该缓存能力。

## 4. 所有权

- Context Engine拥有ContextPackage和CachePlan语义；
- Model Profile声明精确Route缓存能力；
- Invoker映射Provider原生字段并返回Usage/Receipt；
- Runtime只关联Fence、Placement、Remote Continuation和证据；
- Harness只消费ContextPackage/CachePlan，不能为命中率改变语义。

## 5. 输出

- ContextPackage、内容/来源/权限/裁剪说明；
- CachePlan、Partition摘要、Retention和失效条件；
- Token/容量报告、Provider Usage与成本节省证据；
- 构建轨迹，不默认保存敏感正文。

## 6. 进入Context自身Plan的门槛

- Context来源图、组合与冲突规则；
- ContextPackage不可变Schema；
- Cache Partition、TTL、Retention和Provider能力；
- 缓存污染、权限漂移和语义重排反例；
- 用户确认测试语料、隐私和首期算法范围。
