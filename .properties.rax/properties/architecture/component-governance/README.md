# 6+1组件统一运行与治理合同

## 1. 状态与范围

本合同由Harness与Runtime两条基座任务在2026-07-14共同冻结，供以下七条并行组件线共同遵守：

1. Continuity：Timeline、Timestamp、Checkpoint、Rewind、Snapshot、恢复与持久化；
2. Tool/MCP：Tool、MCP、MCP Server、Tool Use、过程暴露与Review Gateway；
3. Memory/Knowledge：候选、筛选、Knowledge Gateway、正式持久化与进入机制；
4. Sandbox：隔离级别、权限、范围、Lease与Checkpoint联动；
5. Review：人类介入、自动审核、行为资产与后续反馈；
6. Context/Cache：上下文、缓存、注入、上下文/Prompt工程与反馈式版本管理；
7. Harness：共用执行外壳及具体Route接线。

“组件独立运行”精确定义为：语义和状态所有权独立、可以独立部署；当前不预设七个OS进程、RPC、数据库、生产拓扑或SLA。

## 2. 部署四区

| 区域 | 必须放置 | 禁止事项 |
|---|---|---|
| 宿主Control Plane | Runtime Kernel/Control Plane Host、Application Coordinator、最终Governance Dispatch Gateway、Identity/Authority/Fence/Command/Desired State/Activation/Run线性化入口 | 不得随Instance Sandbox死亡；不得吞并组件业务语义 |
| 每Instance Sandbox/Data Plane | Harness Interaction Loop、Run Session、受控Tool Runner/MCP Client、最终Prompt组装/注入、Sandbox Enforcement Agent、Run内临时Worker | 不得持有跨Instance权威事实或长期明文Secret |
| Sandbox外持久State Plane | Runtime事实、Evidence Ledger、Timeline/Checkpoint Manifest与加密Snapshot、Memory/Knowledge/Asset、Context Cache/Prompt资产、Review Verdict/行为资产、Tool Registry/MCP Manifest、Sandbox Lease/Placement | Sandbox本地盘不得成为唯一权威副本；必须按tenant/identity/authority/lineage/instance/run/effect分区 |
| Remote Provider | Model、Hosted Tool/MCP Server、远程Sandbox、对象/向量/知识/缓存服务、远程Review模型 | 只能作为绑定Provider经Port和宿主Gateway进入；自报只能是Observation/Receipt；必须有短期凭据、最小Scope、Inspect/Settlement、Residual与Conformance |

## 3. 七任务默认放置

- Continuity：Barrier/Restore协调在宿主；参与者Snapshot在各Data Plane；Manifest、Timeline和Snapshot Blob在State Plane；允许远程对象库。
- Tool/MCP：Registry、Review/Effect Gateway在宿主；Tool Runner/MCP Client在Sandbox；允许远程MCP Server；Manifest、Intent、Receipt在State Plane。
- Memory/Knowledge：候选可由Data Plane产生；筛选/Commit Controller在Sandbox外组件服务；正式Memory/Knowledge在State Plane；远程检索或存储外发属于Effect。
- Sandbox：Lease/Placement/Policy Controller位于宿主或受信外部控制服务；Enforcement Agent位于Data Plane；Lease/Fence/隔离证据位于State Plane；允许远程Sandbox Provider。
- Review：Verdict Owner、人类介入入口和最终审核Gateway位于Sandbox外；自动Review Worker可隔离或远程运行；Verdict/行为资产位于State Plane；Review只判定，不Dispatch或Commit。
- Context/Cache：Controller位于Sandbox外；最终Run级Prompt组装/注入可位于Data Plane；Cache/Prompt资产位于State Plane；远程Context/Cache必须通过Disclosure Effect与分区门禁。
- Harness：每Instance执行外壳位于Sandbox/Data Plane或受控远程Execution Surface；宿主只放ExecutionPort Adapter与监督入口；Run Session归Harness，Checkpoint贡献进入State Plane；Harness不拥有Runtime Run或Outcome。

## 4. 单一语义所有者

- Runtime Kernel：拥有Instance三维状态、Reconcile/Binding协调、Run Fact/ExecutionOutcome、Fence后的失败真实性和Cleanup聚合；不拥有模型、工具、上下文、记忆或审核算法。
- Runtime Control Plane：拥有Lineage/Instance、IdentityExecutionLease、Command/Desired State、Admission/ActivationCommit、Fence Epoch和Runtime线性化事实入口；不执行组件业务动作，不替Budget、Review、Memory等领域作结论。
- Application Coordinator：编排跨域调用、摄取Claim、请求独立Inspect/Review/Settlement，把用户工作流映射为Runtime命令；不直接写Kernel事实，不成为领域Owner，不绕过Gateway。
- 组件Controller：拥有自身Descriptor/Capability、领域状态机、内部持久状态、Receipt/Inspect语义与资源释放；在实际执行点重验Fence；不拥有全局Identity/Instance/Run，不修改其他组件事实。

## 5. 十条统一门禁

1. 只通过版本化Port接入。Manifest至少包含ID、Kind、Version、Artifact Digest、Contract Version、Locality、依赖DAG、Capability TTL、Conformance、Residual、Effect Owner和Cleanup Owner。
2. 记录必须携带Tenant、Identity+Epoch、Lineage+Plan Digest、Instance+Epoch、适用SandboxLease+Epoch、Run ID、Authority Epoch；旧Epoch只能成为迟到证据。
3. 模型外发、网络、Tool/MCP、Hosted Tool、资源、Cache写、Credential、Memory/Asset正式提交全部先持久化EffectIntent，并绑定Payload Digest、Revision、Risk、Scope、Budget、Review、Idempotency和Settlement Owner；UnknownOutcome只能Inspect，禁止盲重试。
4. 高风险Dispatch采用双重门禁：宿主Gateway重新读取并验证当前Fence、Identity Lease、Authority、Review Verdict、Budget、Scope和Intent Revision；实际Sandbox/Invoker/Tool/Commit执行点再次验证。任一权威事实不可访问且无显式Offline Policy时Fail Closed。
5. Evidence使用Source Identity/Epoch/Sequence、Payload Digest、Causation/Correlation和Ledger Scope/Sequence。Observation、Attestation和Authoritative Fact不得自动升级；同序号重放幂等，换内容产生EvidenceConflict。
6. Harness Completed/Ready/Cleaned及组件成功回包都只是Claim/Observation；Runtime或领域Owner独立Inspect后才能CAS提交权威结果。ExecutionOutcome不等于Task、Goal或Artifact成功。
7. Recovery必须使用Write-Ahead阶段化Journal；回包丢失先Inspect；Checkpoint Partial只作诊断；Rewind/Restore创建新Instance与更高Epoch，不宣称外部世界回滚；Remote Residual、Unknown Effect和Cleanup分别报告并占用冲突域。
8. Review Verdict绑定精确Candidate/Intent Payload Digest、Revision、Scope、Authority和TTL；任一漂移必须重审。Review只判定，Management只提出意图，最终Dispatch/Commit由对应Gateway/Owner完成。
9. Conformance统一使用`fully_controlled`、`restricted_controlled`、`contained_observe_only`、`rejected`。声明外能力不得暴露；不可Fence的持久Effect、长期明文Secret、不可控网络直接Rejected。
10. 每组件必须具有合同、单元、白盒、黑盒、故障注入、Race、Vet和Conformance测试；集成前通过同一Foundation闭环，不得以Fake成功冒充生产Backend或SLA。

## 6. 并行写入规则

- Runtime任务独占`ExecutionRuntime/runtime/**`及Runtime设计、计划、模块和Memory资产。
- Harness任务独占`ExecutionRuntime/harness/**`及Harness对应资产。
- Model Invoker任务独占`ExecutionRuntime/model-invoker/**`及对应资产。
- 其余六条组件线只能写自己的新实现目录及同名设计、计划、模块、Memory目录。
- 七任务不得直接修改Runtime Core/Ports或Harness公共合同。发现缺口只回传Port Delta：用例、Owner、输入输出、不变量、Effect/Recovery、反例和兼容影响，由对应Owner串行合入。
- `.properties.rax/MAIN.md`、全局Properties/Architecture索引、Go Workspace、CI和根配置由单一集成任务串行修改；组件任务只回传索引增量。
- 禁止跨组件导入实现包、复制共享类型、并发覆盖其他任务文件，或预选未经确认的生产DB、RPC、进程拓扑和SLA。

## 7. 冻结结论

Runtime只托管共享执行治理与线性化Runtime事实，不承载组件业务语义；每组件语义/状态独立并可独立部署，通过Port接入；高风险动作最终Dispatch前由宿主Governance Gateway重验Fence、Review、Budget和Scope，实际执行点再验一次。双重门禁不得降级为只信Remote Provider或只信Sandbox。
