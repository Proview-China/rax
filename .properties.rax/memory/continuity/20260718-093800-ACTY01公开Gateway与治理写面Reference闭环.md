# A-CTY-01公开Gateway与治理写面Reference闭环

时间：2026-07-18 09:38 CST

## 结论

用户授权最小跨Owner Delta后，A-CTY-01的reference代码闭环已完成：Application公开coordinate-only Continuity Workflow DTO、trusted Assembler Port、`Submit+Inspect` Gateway；Continuity只经Application公开`contract/ports`接入，并提供SDK治理写请求和CLI命令映射。

这不是production GO。真实CompiledGraph/Binding/Consumer current Assembler、各kind Descriptor/Schema/Capability route、根CLI/API、Restore Stage/Activate及任何Provider仍未实现，Provider调用保持0。

## 已实现

- 七类closed kind：Timeline Projection、Checkpoint Create、Fork、Rewind Plan、Restore、Attach Artifact、Resolve Retention；
- public Request只含stable identity、exact Scope、Continuity-owned Domain Request Ref、expected revision、CompiledGraph/Binding/Consumer refs与时间边界；无raw Bundle、Permit、Review、Provider、可信current/sequence或Runtime Outcome；
- Application Gateway验证trusted Assembly对Request digest、Scope、Idempotency、canonical inline Request、root kind/step的exact绑定后，才复用既有Facade；
- Submit回包丢失沿既有Submission/Command/Outbox exact Inspect恢复；Inspect可在Journal创建前返回exact refs，Journal出现后返回exact Journal ref；
- Continuity Adapter只依赖Application公开contract/ports，并要求Domain Request Owner为`praxis/continuity`；
- SDK只新增`Submit/InspectGovernedWorkflow`，CLI只新增Continuity-owned命令描述、strict JSON参数和结果映射，不注册根CLI。

## 验证

- Application定向ordinary100与race20：七kind、assembly drift、lost reply、same-ID drift、64路exact replay、typed-nil；
- Continuity Adapter/SDK/CLI定向ordinary100与race20：Owner拒绝、clone/no-alias、strict decode、command-kind mismatch、64路并发确定性；
- Application与Continuity两个模块的full ordinary、full race、vet全部PASS；gofmt、公开导入边界、Markdown links、drawio XML、semantic stale、whitespace/diff-check全部PASS。

## 继续NO-GO

- production trusted Assembler/current Readers/各kind route；
- 根CLI/API注册与endpoint/credential；
- production Checkpoint Participant/capture/root；
- Restore Execute/Stage/Activate、新Instance实际创建；
- remote blob/purge/archive及任何真实Provider；
- 外部世界回滚；Partial仍只诊断，legacy接口不得扩权。
