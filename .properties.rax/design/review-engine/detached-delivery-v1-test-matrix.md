# Review Detached Delivery V1 测试矩阵

当前是资产级 oracle；外部 Owner Port/root 未冻结前不得用fixture宣称production通过。

| ID | 场景 | 预期 |
|---|---|---|
| DET-01 | 64并发同detached canonical | 一个child Run/lineage/Binding；其余同ref或Inspect-only |
| DET-02 | Inline Request | 零child Run、零Binding、零Closure |
| DET-03 | 同Binding ID换Target或Delivery | Conflict；零写 |
| DET-04 | Parent Phase在S1/S2漂移 | 零Binding、零child start |
| DET-05 | Target/Scope/Tenant漂移 | Fail Closed；零Binding |
| DET-06 | Lineage指向错误parent | Fail Closed |
| DET-07 | Child Run identity/scope/plan漂移 | Fail Closed |
| DET-08 | Application coordination不匹配 | Fail Closed |
| DET-09 | Child current NotFound/unknown | 零start、零Binding |
| DET-10 | Request为唯一最短TTL | Binding Expires精确等于Request |
| DET-11 | Phase/Target/Waiting/Lineage/Child/Coordination逐项最短TTL | table-driven逐项精确min |
| DET-12 | TTL在S1/S2穿越 | 零CAS |
| DET-13 | Reader内部clock rollback | zero Binding/Closure |
| DET-14 | Pending Run create丢回复 | 只Inspect同RunID；不换ID |
| DET-15 | Start边界丢回复 | 只Inspect原attempt；物理start最多一次 |
| DET-16 | Binding create丢回复 | 只Inspect原canonical Binding |
| DET-17 | Child Completion Claim到达 | 不产生Verdict或父allow |
| DET-18 | Child closed但Attestation缺失/漂移 | 父继续等待；零Verdict/Closure |
| DET-19 | current Attestation+Verdict+terminal/settlement | 唯一Closure；父仍需独立Gate current |
| DET-20 | Cleanup unresolved/迟到输出/Target supersede/外部approve评论 | 不恢复父Run；历史+Residual；Inspect-only |
| DET-21 | Runtime/Thread arm同时有或同时空 | InvalidArgument；零写 |
| DET-22 | S1 Resolve unknown | 新完整cut；不得冒充原结果恢复 |
| DET-23 | S2 exact Inspect unknown | bounded detached同Ref一次；仍unknown则Fail Closed |
| DET-24 | current index ABA回到旧full Ref | Conflict；旧historical仍可读 |
| DET-25 | Cross-tenant相同local ID | 隔离；不得冲突或串读 |
| DET-26 | External delivery成功但无Settlement/Observation | 零Closure、零Attestation升级 |
| DET-27 | Platform comment写“approve” | 仅Observation；零Verdict |
| DET-28 | Child Run Outcome成功但Review拒绝 | Closure可记录两者，父Gate deny；不改Outcome |
| DET-29 | Closure create丢回复 | exact Inspect原Closure；不重做Reviewer/Provider |
| DET-30 | deep-clone mutation返回值 | repository历史/current不受影响 |
| DET-31 | Review domain import Application/Harness/platform实现合同 | import scan失败；只允许Review合同+Runtime public core/ports与中立source coordinate |
| DET-32 | 中立coordinate被Review重算或当current事实 | Fail Closed；必须由来源Owner Reader逐字段复读 |
| DET-33 | Binding/Closure canonical literal golden | ID与Digest逐字匹配冻结domain/contract/type/body |
| DET-34 | IdentityInput单字段缺失或JSON tag改名 | InvalidArgument；零写 |
| DET-35 | Ref.Digest、顶层Digest或body任一漂移 | Conflict；零写 |
| DET-36 | Store同ID同body重复create | 同deep clone；历史一项 |
| DET-37 | Store同ID换body | Conflict；原exact仍可读 |
| DET-38 | Store staged failure | Binding/Closure均零泄漏 |
| DET-39 | create commit后丢回复 | 只Inspect原Ref；mutation调用一次 |
| DET-40 | recovery timeout>2s或超过剩余TTL | constructor拒绝或运行时裁剪；不得越过TTL |

门矩阵：Owner-local contract/unit/whitebox；cross-owner blackbox/fault/conformance；`targeted -count=100`、`race -count=20`、full ordinary/race/vet、import DAG、composition system test。DET-33必须使用字面量golden而非调用同一Derive自证。所有命令必须实际运行后才能登记PASS。
