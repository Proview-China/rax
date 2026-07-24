# Memory/Knowledge联合冻结与reference集成闭环

> 状态：**completed / implementation_software_test_yes**。Owner-local与cross-owner reference integration均为P0=0/P1=0/P2=0。真实远程Effect、生产Backend与production per-turn root仍NO-GO。

## 事件

以`tmp.document/Memory&Knowledge.md`为最高业务输入，完成backend-neutral Memory/Knowledge框架与必要cross-owner reference链的联合冻结和实现：

```text
Harness committed PendingAction exact Session/Turn
  -> Harness public Application mapping
  -> Application Prepare
  -> Memory/Knowledge Owner S1
  -> Context pending DomainResult/Manifest/Frame/Generation seal
  -> Context create-once TransitionProof seal
  -> Memory/Knowledge Owner S2
  -> Context atomic local ApplySettlement + Generation current CAS
  -> exact Frame(memory_recall + knowledge_reference) publish
  -> Application Inspect original attempt on lost reply
```

Application只协调；Context唯一拥有TransitionProof、Frame、Generation与Context current；Memory和Knowledge分别拥有自己的Attempt/current/Record/Source/Snapshot、stable/fresh projection与正文currentness；Harness只提供exact Turn映射并消费published exact Frame。

## 本轮关键收口

- Application公共三阶段Port、exact Turn配对、S1/S2 stable association与lost-reply single-call gate；
- Context create-once TransitionRequest/Proof、InspectExact/history、`knowledge_reference`、最终Apply/Settlement/CAS对`ProofRef + StableSourceSetDigest + S2AssociationSetDigest`的同锁域exact绑定；
- Memory/Knowledge两个V2 Adapter直接复用live Owner Reader nominal，不建第二DTO、Store或current；SessionEvidence/TurnEvidence与Application applicability逐字段绑定，跨Turn replay与Session substitution拒绝；
- Memory=1、Knowledge=1的real V2 Owner Reader reference fixture发布含`tool_result`、`memory_recall`、`knowledge_reference`的exact Frame；
- `ContextTurnSourceCurrentV1`、Owner Source Request/Envelope拒绝结构合法但属于另一Turn的exact ref替换；Session Fact与SessionApplicability保持Harness定义的两个独立exact ref，不伪造digest相等关系。

## 实际验证

```text
Memory/Knowledge:
  go test -count=100 ./...                         PASS
  go test -race -count=20 ./...                   PASS
  go test ./...                                   PASS
  go test -race ./...                             PASS
  go vet ./...                                    PASS
  whitebox/blackbox/fault/conformance定向命令       PASS

Application:
  go test -count=100 ./contract .                 PASS
  go test -race -count=20 ./contract .            PASS
  go test ./... / go test -race ./... / go vet    PASS

Context:
  go test -count=100 ./contract ./kernel ./applicationadapter ./tests/integration       PASS
  go test -race -count=20 同组                                                        PASS
  go test ./... / go test -race ./... / go vet                                          PASS

Harness:
  go test -count=100 ./applicationadapter -run TestContextTurnSource       PASS
  go test -race -count=20 同组                                            PASS
  go test ./...                                                            PASS
  go test -race ./...                                                      PASS
  go vet ./...                                                             PASS（共享tool-mcp短暂编译漂移恢复后复跑）

资产/边界:
  Markdown relative links                         PASS
  Memory/Knowledge Draw.io XML                    PASS
  git diff --check                                PASS
  gofmt -d                                        PASS
  import-boundary / Owner Reader zero-network     PASS
```

## 保留边界

- Current Reader没有`Retrieve`、Provider、Resolver、远程正文或Checkpoint/Restore能力；
- Retrieval Gateway在retrieval-specific Applicability/Evidence/Settlement版本联合冻结前保持unsupported，Provider/Resolver=0；
- reference store/fixture不代表生产State Plane、数据库、容量、SLA或production root；
- production per-turn root必须由装配Owner显式接线并单独完成系统验收；本事件不预选DB、Vector DB、Graph DB、RPC、进程拓扑或SLA。
