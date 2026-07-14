# Runtime Review Verdict V2设计

## 定位与单主

Review组件是唯一Verdict Owner。Runtime只保存、校验并投影Review权威事实，不替Review作判断，不派发Provider，不提交领域结果，也不修改Run、Identity或组件事实。

旧`ReviewPort/VerdictObservation`仍只是Observation。V2把可信层级拆开：

1. `ReviewCandidateV2 + ReviewCaseFactV2`：绑定已write-ahead的Effect subject，初态`pending`；
2. `ReviewAttestationObservationV2`：人类、自动或远程Reviewer证据，不是Verdict；
3. `ReviewVerdictFactV2`：Review Owner对Case的单一CAS决定；
4. `ConditionSatisfactionFactV2`：conditional所需的独立、机器可验证满足事实；
5. `DispatchReviewFactV2`：Gateway使用的当前只读投影，不是第二Verdict主库。

## 无环两阶段绑定

权威顺序固定为：

`Review subject摘要 → Review Policy Fact → Candidate摘要 → Effect ReviewBinding → Case → Verdict/Satisfaction → Dispatch投影`

`ReviewSubjectDigestV2`保留预分配Case Ref/Revision及所有非Review治理字段，但排除之后才能生成的Candidate、Review Policy和Dispatch Policy摘要。nil/empty集合按逐字段合同归一化，SandboxLease按值而非Go指针地址比较。最终`NewProposedEffectFactV2`仍要求所有真实摘要存在。

## 生命周期与当前性

- Case：`pending → decided | expired | revoked`；
- Verdict：`accepted | rejected | conditional → expired | revoked`，决定不可原地翻转；
- Satisfaction：`pending → satisfied → expired | revoked`，已线性化Proof在失效后保留且不可改写；
- Create/Decide仅允许目标Effect处于`proposed|accepted`；
- Dispatch投影仅在Provider触达前对`accepted|dispatch_intent`可读；
- `dispatched|unknown_outcome|settled`不能重新取得Review授权。

每次Create、Decide、Satisfy、Gateway Issue、Gateway Begin及实际执行点都重新读取Policy、Authority、BindingSet、CurrentScope/Run和必要Satisfaction。任一ID、revision、digest、scope、epoch、state或TTL漂移均失败关闭。

## conditional、自动Reviewer与自审

- conditional必须使用namespaced Condition Schema/Constraint，逐条件绑定Satisfaction Owner的Binding member、Capability、Authority、Scope、revision与TTL；自由文本不授权；
- Permit精确绑定Verdict及Satisfaction Ref/Digest/Revision；同一Verdict不允许替换Satisfaction。证明撤销或过期后必须创建新的Effect intent revision、Case、Verdict、Satisfaction与Permit，旧Permit永不复活；
- 自动或远程Reviewer调用本身是独立Effect；响应只是Observation，只有exact relation、Provider、payload和`confirmed_applied` Settlement经Inspect后才可支持Verdict CAS；`unknown_outcome`只允许Inspect；
- Actor与Reviewer主体相同时默认拒绝，只有精确current Policy Fact的`allow_self_review`允许；
- `operation_not_required`只能来自显式current Policy Fact，且不得伪造Reviewer invocation。

## 执行点门禁

Permit Verifier在Provider触达前自行复读Binding、CurrentScope、Credential、Authority、Review/Verdict/Satisfaction、Budget和Dispatch Policy。调用方传入的`DispatchCurrentFactsV2`只是待比对快照，不能自授权；任一Reader不可达且无未来明确离线策略时失败关闭。Verifier Receipt仍只是Enforcement evidence，不证明Provider已调用或领域已提交。

## 兼容与边界

- 不提升旧Observation的权威性；
- 自定义Reviewer/Condition只使用P0.1 namespaced schema、capability和opaque evidence，不获得Runtime内部句柄；
- P0.3只绑定稳定Evidence Ref/Digest；P0.4再接唯一Evidence Ledger，不创建第二Ledger Owner；
- 内存fake只用于确定性、故障注入与race测试，不宣称生产持久性、一致快照、RPC或SLA。

## 验收

已覆盖Effect→Case→Verdict→Permit→Begin真实时序、丢回包Inspect、并发相反决定单次线性化、全部current事实漂移、conditional错误proof与撤销、自动Reviewer exact Settlement、自审Policy、显式not-required、canonical nil/empty、SandboxLease值语义、自定义Reviewer不自授生产/派发/提交，以及执行点Provider零触达反例。
