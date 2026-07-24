# G6A Action Matrix/Router V1测试矩阵

## 1. Matrix与canonical

| ID | 用例 | 预期 |
|---|---|---|
| G6A-U01 | 唯一Action矩阵 | `run + praxis.tool/execute + praxis.tool/single-call-action-v1`且Generation、五维全部required |
| G6A-U02 | 其他Operation/Effect/Profile | unsupported；零Evidence写、零Tool写、零Provider |
| G6A-U03 | Generation缺失或五维任一missing/optional/forbidden | Fail Closed；零写 |
| G6A-U04 | source coordinate nominal projection | `Kind/ID/Revision/Digest`逐字段完全相等 |
| G6A-U05 | router重新seal、换ID/revision/digest | 拒绝；不创建Runtime Fact |
| G6A-U06 | Session/Turn source type-pun | 拒绝，即使文本字段相同 |
| G6A-U07 | Action指向PendingAction/Application DTO | 拒绝；必须是Tool `ActionCandidateV2` exact source |
| G6A-U08 | Context指向next Frame或Application coordinate | 拒绝；必须是ParentFrame Owner source |
| G6A-U09 | Boundary Ref Validate/canonical | ID、Revision、Digest缺失或篡改拒绝；同内容deterministic |
| G6A-U10 | Boundary Projection Seal/Validate | exact Ref、Operation/Scope、Attempt、execute Enforcement/Handoff、Stage、TTL全部进入digest |
| G6A-U11 | Boundary stage type-pun | 非`provider_boundary_crossed`、空值或未来stage拒绝 |

## 2. Owner-current Reader

| ID | 用例 | 预期 |
|---|---|---|
| G6A-R01 | Run current | exact Tenant/Scope/Run/revision/digest/current TTL通过 |
| G6A-R02 | Harness Session/Turn S1/S2 | distinct source、waiting_action、PendingAction、短租约全exact |
| G6A-R03 | Tool ActionCandidate current | Candidate与PendingAction/Run/Session/Turn/Effect/Owner全exact |
| G6A-R04 | Context ParentFrame current | Frame/Manifest/Generation/Run/Session/Turn全exact |
| G6A-R05 | 缺Reader、未知Kind、重复Kind路由 | Issue前Fail Closed，调用后端/Provider计数为0 |
| G6A-R06 | 任一source revision/digest漂移 | Issue与Handoff均拒绝，旧public ref不自动升级 |
| G6A-R07 | Reader返回另一Fact或scope | 拒绝，不能只比较digest字符串 |
| G6A-R08 | TTL边界前/边界/过期或Clock回拨 | 边界及过期Fail Closed；不得延长底层TTL |
| G6A-R09 | Model Projection Exact Reader缺失/unavailable/Calls!=1 | Application Request不形成，零Tool写、零Provider |

## 3. phase、恢复与终态边界

| ID | 用例 | 预期 |
|---|---|---|
| G6A-P01 | prepare/execute各自独立Qualification/Handoff/Consumption | 两链均exact且ID/source sequence/4.1 phase不同；Consumption只在响应/Observation后形成 |
| G6A-P02 | phase交换、复用、缺失、重复 | 零Provider、零DomainResult、零Settlement |
| G6A-P03 | 只有4.1 execute current或只有execute handoff | Provider调用数为0 |
| G6A-P04 | execute Enforcement/Handoff exact但Tool boundary未CAS/未current | Provider调用数为0；Runtime不代写Watermark |
| G6A-P05 | Provider Observation冒充DomainResult | 拒绝；零Runtime Settlement |
| G6A-P06 | exact Boundary Ref经注入Reader返回current projection | Ref、Operation/Scope、Attempt、execute Enforcement/Handoff、Stage、TTL全exact后才允许至多一次Provider调用 |
| G6A-P07 | Boundary Reader missing/unavailable/NotFound | 零Provider；Runtime不创建Boundary Fact或回退读取Tool Store |
| G6A-P08 | boundary CAS回包丢失或之后崩溃 | 视为可能已调用；只Inspect原Attempt/Observation，不再次调用Provider |
| G6A-P09 | boundary预填prepare/execute Consumption | 拒绝；Consumption必须在对应响应/Observation后形成 |
| G6A-P10 | lost Issue/Handoff/Consume回复 | 只Inspect原canonical ID，不换ID、不重派Provider |
| G6A-P11 | Reader unavailable/UnknownOutcome | 保持inspect-only，Provider方法入口不增加 |
| G6A-P12 | Tool DomainResult到Settlement V4 | 仍须Tool ApplySettlement后才形成settled ToolResultV2 |
| G6A-P13 | Boundary Ref type-pun或Reader返回另一Ref | Fail Closed；Provider调用数为0 |
| G6A-P14 | Boundary Operation/Scope digest漂移 | Fail Closed；不得只比较Attempt或Effect ID |
| G6A-P15 | Boundary Attempt、execute Enforcement或Handoff任一换ref | Fail Closed；Provider调用数为0 |
| G6A-P16 | Boundary TTL边界、过期或clock rollback | fresh clock拒绝；Provider调用数为0 |
| G6A-P17 | Tool Adapter试图让Runtime生成Boundary ID/digest或写Watermark | import/conformance拒绝；Runtime无写Port |

## 4. G6A停止线与组合

| ID | 用例 | 预期 |
|---|---|---|
| G6A-B01 | 成功N=1 fixture | 仅返回settled ToolResultV2、current V4 Inspection、public Association Inspect |
| G6A-B02 | Capability enable、Context Refresh、Continuation、Turn推进 | 调用计数全部为0 |
| G6A-B03 | N>1或万能Hook | unsupported，完整Observation保留但不执行 |
| G6A-B04 | 无production composition root | 只允许显式test fixture注入；Conformance不得报告production eligible |
| G6A-B05 | 组件Adapter尝试持有Runtime Fact写口 | import/conformance gate拒绝 |
| G6A-B06 | public-only Conformance使用Boundary Reader | 只注入Reader和Provider计数seam；不import Tool、不持有Watermark写口、不声明production eligible |

## 5. 实施门禁

联合`YES`后的代码阶段至少要求：unit、whitebox、blackbox、fault、64并发、lost-reply、ordinary、race、vet、gofmt与diff-check。当前资产阶段只验证Markdown链接、drawio XML、冻结术语与stale claim；不把设计测试矩阵写成已执行结果。
