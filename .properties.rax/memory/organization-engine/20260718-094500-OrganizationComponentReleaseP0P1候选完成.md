# Organization Component Release P0/P1候选完成

## 事件

Organization Owner已按用户确认的Component Release V1设计形成`ExecutionRuntime/organization-engine/release`代码候选：固定`components/organization`身份和`praxis.organization/review-eligibility-current`能力，发布reference-only、SQLite standalone和受严格current约束的production合同。

## 已闭合

- memory/reference无readiness时固定`reference_only`；
- exact SQLite Resource/Schema/Integrity/Restart/Historical Reader/Current Reader共同证明local readiness，最高为`standalone`；
- production只绑定Organization自身local readiness、ResourceBindingSet、cleanup、deployment、executable factory binding和独立certification，不读取Review consumer；
- Runtime仍是Runtime Effect/Settlement Owner，Organization只拥有本域cleanup；
- `ModuleFactoryDescriptorV1`只描述构造合同，不是可执行Factory，也不携带SQLite handle或production root；
- Publisher对readiness执行S1/S2、TTL和时钟复读；回包丢失只按exact ReleaseRef Inspect；同revision升级/换内容冲突；64并发只线性化一个Release；
- Agent Assembler外部黑盒可以exact读取reference candidate；production文件import边界不含Review/Application/Host/internal/storage实现。

## 实际验证

```text
go test -count=100 ./release                                      PASS (1.993s package time)
go test -race -count=20 ./release                                PASS (3.487s package time)
go test -count=1 ./...                                           PASS
go test -race -count=1 ./...                                     PASS
go vet ./...                                                     PASS
release coverage                                                 84.7%
```

## 当前门禁

本事件只表示Organization Owner P0/P1代码候选与软件门完成，仍等待独立代码审计。Review Human Multi-Sign Release variant、真实production readiness producer、ResourceBinding、executable factory、deployment/certification和唯一Host composition root均未闭合；不得宣称Organization production或系统GO。
