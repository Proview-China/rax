# AgentDefinition V1 公共合同与 Reference Owner 实现完成

## 事件

已按确认设计和计划在 `ExecutionRuntime/agent-definition` 落地独立 Go module：

- public Source/Definition/Ref/current 合同；
- 首版七核心 kind required + production 门；
- 自定义 namespaced kind/capability/extension Catalog；
- strict YAML/JSON decoder 与 canonical digest；
- Approval current-before-seal Service；
- immutable history、current CAS、revoke/expire、lost-reply恢复；
- 线程安全 Memory reference store 与第三方 Conformance fixture；
- 单元、白盒、黑盒、故障、64并发、fuzz、race、vet、import boundary测试。

## 边界

本模块只完成 Definition Owner 公共合同和 reference 实现。Memory Store 不是
生产持久 Backend；Agent Assembler、Harness Assembly、Runtime binding/admission
与唯一 Host Composition Root 仍由各自模块完成，不能把本事件解释为完整 Agent
已经生产可运行。

## 依赖裁决

模块单向复用 Runtime `core` canonical/error/SemVer，Runtime 不反向依赖，未形成
SCC；未新增独立公共 canonical 模块，也未复制摘要算法。

## 独立复核返修

- 首版核心 kind 改为私有数组，公共 `RequiredCoreKindsV1()` 每次返回 copy；
- Extension payload 在规范化前先走 Runtime strict JSON duplicate-key 扫描；
- SecretID 冻结为 namespaced identifier，并拒绝 traversal、绝对路径、反斜杠和 file URI；
- 新增相应 mutation、duplicate-key 与四类路径反例。

## 最终验证

- ordinary `go test -count=1 ./...`：最终代码树连续100轮通过；
- race `go test -count=1 -race ./...`：最终代码树连续20轮通过；
- decoder fuzz 3秒：109491次执行通过；
- canonical fuzz 3秒：36327次执行通过；
- full ordinary/race shuffle、`go vet ./...`、gofmt、import boundary、
  `git diff --check`全部通过。
