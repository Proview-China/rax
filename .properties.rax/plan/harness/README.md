# Harness Plan入口

## 当前状态

- Harness公共合同与组件中立最小骨架已经实现并完成验收；
- 多租户/多Run分区、Event丢回包Inspect、模型Unknown安全停留、显式Binding V2身份和关闭后Cleanup观察已补齐；
- Application V3 governed Domain桥已闭合；legacy Intent/Fence路径仍只允许作为兼容测试基座，不可解读为生产dispatch；
- 首切面不选择具体生产Harness、进程协议、账号或真实模型；
- 设计事实源：[Harness设计入口](../../design/harness/README.md)。

## 当前计划

- [Harness公共合同与最小可运行骨架v1（已完成）](./harness-v1.md)
- [Harness Governed V2接线（执行中）](./harness-governed-v2.md)
- [Harness Route绑定与公共引擎接线v1（执行中）](./harness-route-engine-v1.md)
