# MCP受治理Discovery Page与Snapshot只读面软件验收YES

2026-07-18，Tool/MCP owner-local受治理Discovery Page链完成软件验收：Runtime专属
`praxis.mcp/discover` Page gate、Tool canonical Page Command、official MCP Go SDK单页actual-point、
Protocol Receipt、正式Provider Observation/Evidence、typed Page DomainResult、Runtime Settlement V4、
Tool ApplySettlement与Capability Snapshot聚合已经闭合。

Snapshot聚合只接受全部已Apply且terminal的namespace page；Connect Receipt、Page Receipt、
Provider Observation、Applied Current、cursor与Connection任一漂移都会Fail Closed。Go SDK/API新增
exact-current S1/S2只读入口，CLI新增`mcp snapshot --id --revision --digest`，均不触发Discovery或
暴露raw Session/Provider。

Capability Snapshot Repository同步闭合immutable history/current：revision 1 create，successor
必须携full expected-current且只允许current+1 CAS；lost reply重复winner幂等，历史回退、same
revision换内容、ABA与64并发冲突均Fail Closed。

实际门：Discovery Snapshot aggregator ordinary×100、race×20；SDK/API/CLI Snapshot只读
ordinary×100、race×20；Tool模块full ordinary、full race、vet、gofmt、import boundary与
`git diff --check`全部通过。

该YES仅覆盖owner-local实现与软件测试。Application多namespace/多page编排、list_changed到新
Operation调度、production Credential/Network/durable State Plane/composition root/backend及SLA
仍未闭合，不构成system或production GO。
