# ComponentFactoryV2 模块

## 组成

- `contract/component_factory_v2.go`：Factory、Conformance、Registry、Attempt、Instance 与 Preflight sealed objects。
- `contract/component_factory_v2_test.go`：Start/Inspect Attempt identity 与 Schema MediaType canonical反例。
- `ports/component_factory_v2.go`：Owner-declared executable interface、最小 Handle、只读 current readers、Registry 与 Preflight interfaces。
- `registry/component_v2.go`：密封 exact Registry。
- `composition/component_factory_preflight_v2.go`：只读 S1/S2 Preflight。

## 使用方式

先由各 Component Owner提供 Factory，并发布 Descriptor 与 Conformance Current声明；Agent Host仅按typed-nil、exact坐标、字段闭集和sealed no-drift规则结构注册metadata。Implementation、ProviderAccess、ReferenceOnly、Trust与Evidence不证明实现来源/provenance，Registry也不识别fixture/internal/testkit；Registration固定为`ProductionEligible=false`。随后 Preflight 以 H1 Deployment exact current为入口，读取并封存 Builder 权威完整Selection与Package closure，从closure manifest唯一读取 `ModuleFactoryDescriptorV1`，再核对并封存Deployment、Conformance、Resources与Dependencies权威current。Receipt的Checked时间是Request与全部Owner current checked的确定性最大值，不能早于任何证据。

## 当前状态

Owner-local Internal Preview。真实 Component Factory、typed handle resolver、Cleanup Owner链、未来Build/Release Trust Owner与 HostV3 composition尚未接入。当前没有实现来源/provenance验证，也没有durable preflight/Start Store；本地test factory可自报production形metadata并结构注册，但不得production composition。进程外create-once、lost-reply resolution与restart-safe恢复均不提供，因此production保持NO-GO。
