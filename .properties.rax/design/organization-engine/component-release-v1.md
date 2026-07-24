# Organization Engine Component Release V1 设计候选

## 1. 状态与范围

- 状态：用户已于2026-07-18确认设计；Organization Owner P0/P1代码候选与软件测试已完成，等待独立代码审计。Review依赖variant、production readiness与Host root仍未完成。
- 目标：把已经通过Owner-local测试的Organization Review Eligibility current与SQLite State Plane发布为Agent Assembler可读取的声明式Component Release，并为Human Multi-Sign Review提供显式依赖边。
- 本设计不把SQLite单机后端冒充HA/SLA，不建立production root，不签发Runtime Authority或Review Verdict。
- 图：[Component Release V1](component-release-v1.drawio)。

## 2. 固定身份与能力

| 字段 | 值 |
|---|---|
| ComponentID | `components/organization` |
| Kind | `praxis/organization` |
| Contract | `praxis.organization/review-current` `1.x` |
| Capability | `praxis.organization/review-eligibility-current` |
| Port | `praxis.organization/port/review-eligibility-current` |
| Locality | Host Control Plane + sandbox外State Plane |
| Offline policy | denied，除非未来独立Policy明确允许只读缓存 |

公共Port只映射现有`ReviewEligibilityCurrentReaderV1`的Resolve/Inspect语义。Release不得复制Review nominal DTO、不得反向import Review实现，也不得把Organization current升级为Review authorization。

## 3. Support mode

```text
memory/reference store only
  -> reference_only

exact SQLite local readiness
  -> standalone

SQLite durability + public Organization Reader + ResourceBinding
+ cleanup + deployment attestation + executable factory
+ independent certification + Host current
  -> production
```

Production Readiness只绑定Organization自身的exact Release/Manifest/Artifact、SQLite schema/integrity/restart evidence、historical/current Reader、ResourceBindingSet、cleanup current、deployment attestation、executable factory binding和独立certification；S1/S2全字段一致且TTL取最小值。Review consumer adapter current不属于Organization readiness，必须由Human Multi-Sign Review variant或Host SystemReady另行验证，避免`Review -> Organization -> Review consumer`环。fixture、memory store、一次`go test`或文件存在不构成production证明。

## 4. Factory与Owner

- `ModuleFactoryDescriptorV1`只声明构造需求，不是可执行factory。
- production `ControlAdapterFactoryV2`只包装Composition Root预先打开并由Resource Owner发布current的SQLite handle；禁止constructor打开文件/DB、读取credential/environment、启动goroutine/process或产生外部Effect。
- Organization拥有Identity/Role/Delegation/Responsibility领域Fact和current projection的提交语义及本域cleanup；Runtime仍拥有Runtime Effect/Operation Settlement，Review仍拥有Case/Vote/Verdict。
- cleanup绑定exact projection/store resource、Inspect Port与residual；回包未知只Inspect，不能把DB close当作领域purge完成。

## 5. Human Multi-Sign条件依赖

Organization在全局profile中是optional；选中`human_multi_sign_v2`时成为hard required：

1. 使用独立Human Multi-Sign Review Release variant；
2. 该Review manifest声明`components/organization` required dependency；
3. RequiredCapability固定为`praxis.organization/review-eligibility-current`且Provider固定Organization；
4. Harness DependencySpec形成Review -> Organization required/fail-closed边；
5. Organization缺失、非production、过期、drift或Reader unavailable时，Binding/SystemReady零写；不得降级automatic/bypass。

Base automatic Review Release不携该依赖。Host/Profile只能消费Release图，不能暗中注入Organization。

## 6. 测试与反例

- canonical/JSON/type discriminator、nil/empty、stable ordering；
- reference-only/standalone/production三态且禁止同revision提升；
- exact SQLite readiness、schema/integrity/restart、S1/S2/TTL/clock rollback；
- Publisher lost reply按同Release Ref Inspect；same-ID drift/ABA/64并发；
- descriptor-only不能进入factory registry；zero-I/O factory/no raw handle bypass；
- Human Review variant缺Organization、错provider、错capability、optional伪装、automatic fallback全部拒绝；
- Organization/Review/Runtime/Host import DAG无SCC；
- ordinary100、race20、full ordinary/race/vet、import/diff/XML/link。

## 7. 当前门禁

设计与计划已获用户确认，`organization-engine/release`代码候选已形成并通过Owner软件门；Review Release variant仍由Review Owner独立实施。当前只有reference-only/standalone候选，没有真实production readiness发布。独立代码审计通过后仍只形成Release/Readiness候选；真实production必须等待H4 ResourceBinding、executable factory、deployment/certification与唯一Composition Root闭合。
