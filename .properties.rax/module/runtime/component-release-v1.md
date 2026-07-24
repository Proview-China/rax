# Runtime Shared Engine Component Release V1模块说明

## 作用

`ExecutionRuntime/runtime/releasecandidate`把Runtime shared-engine当前真实能力封装为Agent Assembler可消费的声明式组件候选，同时显式保留生产缺口。

## 组成

| 类型 | 作用 |
|---|---|
| `ports.RuntimeSharedEngineComponentIDV1` | Runtime/Application共享的唯一公共ComponentID |
| `BuilderV1` | 构造sealed reference-only Release |
| `ConformanceV1` | 区分public facts/gateways/partial SQLite与完整生产Runtime |
| `ReadinessV1` | 固定列出全部production proof和P0 |
| `PublisherV1` | lost reply后只exact Inspect |
| `ModuleFactoryDescriptorV1` | 描述未来构造入口，不是可执行Factory |

## 边界

- Foundation、fakes、Conformance candidate和partial SQLite都不能授予production；
- Application可只导入`runtime/ports`中的ComponentID，不导入releasecandidate；
- Runtime不导入Application、Harness实现、6+1或Host；
- production promotion、TTL/clock/drift、typed-nil与Evidence缺失全部Fail Closed。

设计见[Component Release V1](../../design/runtime/component-release-v1.md)，实施清单见[Plan](../../plan/runtime/component-release-v1.md)。
