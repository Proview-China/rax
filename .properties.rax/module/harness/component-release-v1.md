# Harness Component Release V1模块说明

## 作用

`ExecutionRuntime/harness/releasecandidate`把Harness现有owner-local能力封装成Agent Assembler可消费的声明式组件发布候选。

## 组成

| 类型 | 作用 |
|---|---|
| `BuilderV1` | 校验Evidence、九项Plan Artifact、TTL和时钟，构造sealed Release |
| `ConformanceV1` | 固定区分owner-local已实现与production未闭合 |
| `ReadinessV1` | 列出完整production proof及当前全部P0 |
| `PublisherV1` | Ensure丢回包后只按exact Ref Inspect |
| `ModuleFactoryDescriptorV1` | 描述未来可信构造入口，不是可执行Factory |

## 使用边界

- 当前Release固定`reference_only`；
- Factory descriptor不持有Store、Route、Provider、Model、Tool、Application或Context实例；
- owner-local current、Route candidate、CommitGate绿测及test fixture都不是production proof；
- 任何TTL过期、时钟回退、返回Release漂移、Evidence/Plan role不完整都Fail Closed。

设计事实源见[Harness Component Release V1](../../design/harness/component-release-v1.md)，执行清单见[实施计划](../../plan/harness/component-release-v1.md)。
