# Review Human Enterprise Multi-Sign V2 实施计划

## 状态

- 业务语义已由用户冻结：租户 K-of-N、必需角色、veto Reject硬否决、显式 Delegation current、production禁自审。
- Review-owned contract/store/current/owner/service 与测试已经实现并纳入最终独立复审 **YES（P0/P1/P2=0）**；真实 Runtime/Policy/Authority/Organization production Adapter、公共 Gate 与宿主 root 仍未关闭。

## 实施 DAG

```text
Policy quorum current Reader ---------+
Authority Identity/Role/Delegation ---+--> Review V2 contract/store
Responsibility Subject current -------+          |
Binding/Scope/Evidence current --------+          +--> panel worker
                                                    +--> quorum owner
                                                    +--> Verdict V2 owner
Runtime Review Authorization V5 -------------------> production Gate/root
```

无 import SCC：Review 只依赖各 Owner public ports；Owner 不导入 Review implementation；Application/Harness 只通过 Runtime V5 与 Review public read ports接线。

## 阶段与文件落点

| 阶段 | Review 文件 | 产出/门禁 |
|---|---|---|
| 1 合同 | `contract/human_multisig_v2.go` | Panel、AssignmentV2、AttestationV2、QuorumDecision、VerdictV2、exact refs、seal/validate/digest |
| 2 Store Port | `ports/human_multisig_v2.go` | compound create panel、record vote、decide quorum、exact inspect/history/current；无外部Owner写口 |
| 3 reference store | `memory/human_multisig_v2.go`、`storage/sqlite/human_multisig_v2.go` | memory仅test；SQLite同事务全有全无、generation CAS、restart recovery |
| 4 current 聚合 | `multisigcurrent/source_v2.go` | Review-owned snapshot + Policy/Authority/Delegation/Responsibility/Binding/Scope/Evidence S1/S2/min TTL |
| 5 Owner | `multisigowner/panel_v2.go`、`quorum_v2.go`、`verdict_v2.go` | K-of-N、roles、veto、waiting映射、CAS/lost reply |
| 6 service | `service/human_multisig_v2.go` | Panel/Assignment/Attestation/Quorum只读与受控命令；无direct verdict |
| 7 API/SDK/CLI | `api/http`、`sdk/go`、`cmd/praxis-review` V2加法 | exact Panel/Assignment请求，strict JSON，tenant/identity隔离 |
| 8 Runtime | 其他Owner独占目录 | Runtime V5 public current/authorization + conformance；未关闭前不发布production Gate |
| 9 root | production composition | SQLite State Plane、五类current、V2 worker、Gate；无Fake/全局mutable registry |

## 验收

1. unit：全部canonical/shape/state/TTL/role/quorum/veto/delegation/self-review反例；
2. whitebox：compound transaction staged failure、history/current/highest、CAS、ABA；
3. blackbox：2-of-3、必需角色、conditional、waiting revision/evidence/higher authority、veto；
4. fault：ctx取消、clock rollback、S1/S2 drift、lost reply exact recovery、SQLite restart；
5. conformance：Store、Policy/Authority/Responsibility Reader、quorum owner、Runtime V5；
6. concurrency：64 independent workers/Reviewers只有一个逻辑current revision和一个 Verdict；
7. repeat gates：targeted ordinary100、targeted race20、full ordinary/race、vet、gofmt、diff/import scan；
8. system：真实HTTP/SDK/CLI、重启继续同一Panel、重复平台事件不重复计票、Runtime Gate只消费V5 current。

## 明确不做

- 不把 V1 ReviewerID 填成群组；不让 Auto 票计入 Human quorum；
- 不由 Review 签发 Identity/Authority/Delegation/Policy；
- 不因 SQLite 被选为 v1 backend 宣称 HA、跨节点线性一致或 SLA；
- 不在 Runtime V5 和 production root关闭前宣称 Human enterprise multi-sign production GO。
