# Agent Assembler V1 验收

## 1. 单元与白盒

- SemVer 范围、artifact exact、contract range、capability、locality、owner、residual、credential 和 extension 解析。
- DAG：缺依赖、自依赖、长环、optional 剪枝后 required 失效。
- 6+1 任一 release 缺失、过期、非 production、无 certification 时零输出。
- Component Release、Catalog、Resolution Facts S1/S2 漂移与 TTL crossing。
- 十个 Assembly Plan Ref 完整映射；empty artifact 合法、empty ref 非法。
- deterministic Plan/BindingPlan/AssemblyInput；同 facts 重试不读取新 wall clock。
- 自定义 namespaced 组件接入和未知 required 扩展 fail closed。

## 2. 黑盒

输入合法 Definition + frozen Catalog/Facts：

1. 产出一个 sealed Resolved Plan；
2. 产出 Runtime BindingPlanV2；
3. 产出可由现有 Harness Compiler 接受的 AssemblyInputV1；
4. Harness Compile 产出 sealed Generation/Manifest/Graph/Handoff；
5. Resolve 全程 Provider/Environment/Tool/Model 调用计数为 0。

## 3. 故障与并发

- 64 并发相同 Resolve：一个 plan 事实，多调用者取得 exact 同结果。
- 64 并发同 PlanID 不同 inputs：一个胜出，其他 Conflict。
- Catalog/Resolution Reader unavailable、indeterminate、lost reply：不读取模糊 latest，不隐式重试到新快照。
- resolve create 回包丢失：Inspect exact plan；NotFound 且 Owner 证明未创建后才可重投同 canonical request。
- clock rollback、facts 过期、S1/S2 digest 漂移：fail closed。

## 4. 导入与所有权

- Assembler 不 import 组件实现、Runtime kernel/foundation/fakes/internal 或 Harness kernel/fakes/internal。
- Runtime/Harness/组件不 import Assembler 实现。
- Catalog 没有 raw constructors/provider handles。
- Assembler 不能发布或重签 ComponentRelease。

## 5. 完成条件

- 普通、race、vet、fuzz/property、import boundary、diff-check 全绿；
- 现有 Harness Compiler 对生成输入通过其完整 conformance；
- required 6+1 全部 production 的正向系统样例通过；
- 任一 owner/current/TTL/residual 漂移的反例均在 activation 前失败。
