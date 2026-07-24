# Model Pre-Dispatch Assembly Current V1测试矩阵

状态：**Runtime ports、public Conformance、typed-nil、shape、canonical、currentness与import-boundary反例已实现；双独立代码审计YES（P0/P1/P2=0/0/0），全门PASS**。

| ID | 类别 | 断言 |
|---|---|---|
| `MPAC1-P01` | shape | 七个public declarations（五DTO、Registry exact Reader、Assembly current Reader）字段、JSON tag和签名精确冻结；Current Ref自身含ToolSurface/Profile/Registry/Semantic/Currentness/Checked/Expires/ProjectionDigest |
| `MPAC1-P01A` | registry request | Registry Reader请求只能是完整`RegistrySnapshotRefV1`；ID-only/latest/name/裸digest selector不存在 |
| `MPAC1-P01B` | registry authority | request.Owner与实际Authority Repository不exact、跨Owner同ID、Owner缺失均在读取前Fail Closed |
| `MPAC1-P01C` | registry exact | stored Ref与request的Owner/Version/ID/Revision/Digest任一漂移为Conflict，返回零Ref |
| `MPAC1-P01D` | registry clone | 成功结果为stored Ref deep clone；修改返回值不改变Owner Store或后续Inspect |
| `MPAC1-P01E` | registry current | historical exact存在但current pointer已推进/撤销时PreconditionFailed；current exact才返回clone |
| `MPAC1-P01F` | registry errors | Invalid/NotFound/Conflict/PreconditionFailed/Unavailable/Indeterminate按closed mapping原样返回，Unavailable/Indeterminate不降级 |
| `MPAC1-P01G` | registry capability | Reader不暴露Registry内容、Publish/CAS/mutation；verified Ref不授Provider/Prepared/Permit/Enforcement |
| `MPAC1-P02` | generation | Projection携完整`GenerationArtifactRefV1`，漏Input/Manifest/Graph/Catalog digest拒绝 |
| `MPAC1-P03` | registry | Registry Owner/ContractVersion/ID/Revision/Digest任一漂移拒绝；不存在alias或裸digest补全 |
| `MPAC1-P04` | cross-field | Ref与Projection的Generation/Handoff/BindingSet/Manifest/Conformance/ToolSurface/Registry及Profile/Semantic/Currentness/TTL/ProjectionDigest逐字段exact |
| `MPAC1-P05` | binding | BindingSet fact/semantic/currentness/projection/TTL任一漂移拒绝 |
| `MPAC1-P06` | digest | Semantic、Currentness、Watermark、Projection各自canonical域和无环计算精确；计算Projection时清空Projection自身digest、Ref.Digest、Ref.ProjectionDigest，最终三者exact；重封篡改失败 |
| `MPAC1-P07` | time | `0 < Checked < Expires`；caller延长TTL、clock rollback、过期均Fail Closed |
| `MPAC1-P08` | current successor | historical exact旧revision仍可在Owner内部审计读取，但current pointer已合法推进时公共current Reader必须返回`PreconditionFailed`；不得返回旧Ref |
| `MPAC1-P08A` | current conflict | same ID+same revision换body或digest必须返回`Conflict`；合法next revision只替换current pointer且禁止ABA |
| `MPAC1-P09` | errors | absent/Conflict/PreconditionFailed/Unavailable/Indeterminate使用既有core类别 |
| `MPAC1-P10` | typed nil | nil/typed-nil Registry/Assembly Reader或Owner current依赖在任何consumer推进前拒绝 |
| `MPAC1-B01` | owner | Runtime没有Store/Publish/CAS；Harness才是semantic/publisher/current Owner |
| `MPAC1-B02` | no echo | Harness/Model/Tool不定义第二Ref/Projection、alias、ObjectRef或有损echo |
| `MPAC1-B03` | import | Runtime ports不导入Harness/Tool/Model；三方只导入Runtime neutral ports |
| `MPAC1-B04` | authority | Projection不授Effect、Permit、Prepared、Enforcement、Provider调用或Settlement权力 |
| `MPAC1-F01` | lost publish | Harness publish回包丢失只按exact Ref Inspect；same content幂等、换内容Conflict |
| `MPAC1-F02` | unavailable | Generation/Handoff/BindingSet任一current Reader不可用时零发布/零consumer推进 |

本矩阵已由`runtime/tests/ports/model_predispatch_assembly_current_v1_test.go`及`runtime/conformance/model_predispatch_assembly_current_v1.go`闭合。审计证据覆盖target ordinary `count=100`、race `count=20`、Runtime full ordinary/race、vet、gofmt、diff-check与import-boundary，结果均PASS；这些结果不授Harness publisher/CAS、相邻Owner适配或production root资格。
