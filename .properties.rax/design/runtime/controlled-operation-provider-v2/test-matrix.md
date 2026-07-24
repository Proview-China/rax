# Controlled Operation Provider V2测试矩阵

## 1. public合同与身份

| ID | 用例 | 预期 |
|---|---|---|
| COP2-U01 | Request/Prepared snapshot/current/Entry ref/Result canonical | deterministic；全部治理字段进入对应digest |
| COP2-U02 | caller提交自由Entry ID或同stable key换ID | Request无自由ID；零Entry、零Provider |
| COP2-U03 | Boundary revision/Checked/NotAfter变化 | derived Entry ID不变；同stable key changed-content Conflict，不二次执行 |
| COP2-U04 | Operation/Effect/Attempt/Prepared ID或digest变化 | stable key与derived ID变化；旧Entry不能复用 |
| COP2-U05 | EffectKind/Profile/两Policy缺失或type-pun | Reader前拒绝 |
| COP2-U06 | ProviderBinding.Capability与EffectKind不等 | 零Entry、零Provider |
| COP2-U07 | Result status/error/Observation非法组合 | Validate拒绝；Result不携Authorization/Outcome/DomainResult |
| COP2-U08 | V1 request/ref/result伪装V2 | type/version拒绝；V1保持不变 |
| COP2-U09 | CurrentRef canonical | exact形状七字段；Digest清空自身后deterministic重算 |
| COP2-U10 | Projection canonical | ProjectionDigest清空自身重算；Ref不含ProjectionDigest，无循环摘要 |
| COP2-U11 | Projection Ref的Declaration/Conformance/Matrix/Watermark任一漂移 | Validate拒绝 |
| COP2-U12 | 五个V2新增Route nominal Go类型与既有Matrix Owner | DeclarationRef、ConformanceRef、CurrentRef、Projection、Reader及既有MatrixKeyV3仅来自Runtime `ports`；Harness只实现Reader，不重定义/alias |
| COP2-U13 | public Reader方法签名 | 必须逐字匹配真实Go签名；缺`context.Context`、exact Ref、Matrix参数或typed返回均拒绝 |
| COP2-U14 | DeclarationRef/ConformanceRef exact shape | 字段逐字等于冻结Go struct；无ContractVersion/ObjectKind字段、自由map或optional metadata |
| COP2-U15 | Ref Validate | 空ID/Publisher、零Revision、非法Digest、非法nested DeclarationRef全部拒绝 |
| COP2-U16 | same RouteID/ConformanceID换任一其他字段 | `core.ErrorConflict`；same canonical幂等，absent ID才NotFound |
| COP2-U17 | ConformanceID非`Derive(RouteID, GenerationRef, BindingSetID)`或Revision非Owner单调CAS | Harness Fact Owner拒绝；零Route Current publish |
| COP2-U18 | ConformanceRef重复嵌入Generation/Handoff自由字段 | schema/canonical拒绝；完整事实只留Conformance Fact与Projection |
| COP2-U19 | 四个DTO真实Go字段/JSON tag编译检查 | `core.Revision/core.Digest`、`GenerationArtifactRefV1`、七个`ProviderBindingRefV2`及全部snake_case tags逐字匹配contracts |
| COP2-U20 | Projection使用未定义`GenerationRef/HandoffRef/ActiveRouteCurrentRef`占位 | API/资产stale扫描拒绝；Generation复用既有类型，另两者扁平ID/revision/digest |
| COP2-U21 | ContractVersion非`ControlledOperationProviderRouteCurrentContractVersionV2`或unknown field/free map | Validate/canonical拒绝 |
| COP2-U22 | Handoff/ActiveRoute同ID换revision/digest后重Seal Projection | Conflict；重Seal不能掩盖current drift |
| COP2-U23 | 七个Binding字段角色Capability互换，尤其Reader/Transport/Provider type-pun | `ProviderBindingRefV2.Validate`加field-role expected Capability拒绝；零Entry、零Provider |
| COP2-U24 | Checked零值、Checked>=Expires、TTL crossing | `core.ErrorPreconditionFailed`；零Entry、零Provider |

COP2-U13冻结并编译检查的唯一签名：

```go
type ControlledOperationProviderRouteCurrentReaderV2 interface {
	InspectCurrentControlledOperationProviderRouteV2(
		context.Context,
		ControlledOperationProviderRouteCurrentRefV2,
		OperationScopeEvidenceApplicabilityMatrixKeyV3,
	) (ControlledOperationProviderRouteCurrentProjectionV2, error)
}
```

## 2. Route、caller与Reader

| ID | 用例 | 预期 |
|---|---|---|
| COP2-R01 | Tool Application Adapter经SingleCallToolActionPortV1进入 | Governance request携current typed Route；通过 |
| COP2-R02 | Application直接调用V2 | caller/assembly conformance拒绝；零Entry、零Provider |
| COP2-R03 | Tool以外caller或Tool Binding漂移 | current route/binding拒绝；零Entry、零Provider |
| COP2-R04 | `RouteBindings[]ObjectRef`或自由ObjectRef冒充Route | type拒绝 |
| COP2-R05 | Runtime定义第二Declaration或改变AssemblyInputV1字段/digest算法 | compatibility/conformance拒绝；Harness canonical Declaration唯一 |
| COP2-R06 | stale/mismatched ConformanceRef | 零Entry、零Provider |
| COP2-R07 | Generation或Handoff ref/revision/digest漂移 | 零Entry、零Provider |
| COP2-R08 | BindingSet/currentness或Tool/Gateway/ProviderTransport/Prepared/Boundary/Provider Binding漂移 | 零Entry、零Provider |
| COP2-R09 | ProviderInspect Binding缺失、过期或type-pun | 零Entry、零Provider |
| COP2-R10 | Request RouteCurrent/Declaration/Conformance ref任一不等于fresh Projection | 拒绝 |
| COP2-R11 | 任一Reader unavailable/NotFound/Indeterminate | 零Entry、零Provider；不信request快照 |
| COP2-R12 | Declaration required extension换内容但Manifest/Input digest未对应 | canonical链拒绝；不得只信Declaration digest |
| COP2-R13 | public Declaration/Conformance/Projection出现kernel Runner binding | schema拒绝；Runner不可枚举 |
| COP2-R14 | active-route scan缺失/过期，或同时报告V1与V2 active | 零Entry、零Provider |
| COP2-R15 | Reader expected Matrix缺失或与Declaration/Projection不符 | 零Entry、零Provider |
| COP2-R16 | ProviderTransport current但actual ProviderBinding stale，或反向 | 均拒绝；两Binding不可合并 |
| COP2-R17 | 同CurrentID换Revision或Digest | Conflict；不能降级为NotFound |
| COP2-R18 | 同CurrentID换Declaration/Conformance/MatrixDigest/Watermark | Conflict；零Entry、零Provider |
| COP2-R19 | CurrentID从未存在 | 仅此情况返回NotFound |
| COP2-R20 | Reader使用缩写`Inspect`或缺expected Matrix参数 | public interface/conformance拒绝 |
| COP2-R21 | `CurrentID`不是`Derive(RouteID, MatrixDigest)` | publish前拒绝；零current写 |
| COP2-R22 | post-binding Conformance未成功即publish | 零current写；Runtime/Tool不能发现或使用 |
| COP2-R23 | 同内容重复publish或首写回包丢失 | exact Inspect幂等恢复；不重复CAS |
| COP2-R24 | changed-content publish、Revision回退/跳过或并发CAS | Conflict；仅首个合法Revision线性化 |
| COP2-R25 | 旧Ref、revoked/expired RouteCurrent或Owner unavailable | Fail Closed；零Entry、零Provider |
| COP2-R26 | Tool/Runtime请求latest/list/free discovery | public Port不可表达；只接受composition注入exact CurrentRef |
| COP2-R27 | Watermark漏Generation/Handoff/BindingSet/ActiveRoute/任一七Binding/WiringInventory | canonical/Projection Validate拒绝 |
| COP2-R28 | Invalid输入 | `core.ErrorInvalidArgument`；backend/current Owner零调用 |
| COP2-R29 | absent CurrentID | `core.ErrorNotFound`；仅ID从未存在 |
| COP2-R30 | same-ID drift/type-pun/current mismatch | `core.ErrorConflict`；不得返回NotFound |
| COP2-R31 | expired/TTL crossing/stale binding | `core.ErrorPreconditionFailed`加既有reason；零Entry、零Provider |
| COP2-R32 | Reader unavailable | `core.ErrorUnavailable`；不fallback request快照 |
| COP2-R33 | current/outcome无法判定 | `core.ErrorIndeterminate`；不伪装NotFound/Expired |
| COP2-R34 | 实现新增`ErrExpired`/`ErrUnknown`或不用`core.NewError/HasCategory/HasReason` | import/conformance拒绝平行错误体系 |
| COP2-R35 | Harness重定义Ref/Projection/Reader/Matrix struct或Runtime定义Harness事实struct | import/API conformance拒绝；type Owner与fact semantic Owner分离 |
| COP2-R36 | ProviderTransport与Provider合并、漏closed `provider` role或只解析一层 | Conformance拒绝；零Entry、零Provider |
| COP2-R37 | Transport/actual Provider alias或raw注入绕过Runner | no-bypass全图拒绝；Provider计数0 |
| COP2-R38 | Declaration只含ProviderTransport、漏closed `provider` endpoint/candidate | pre-binding schema拒绝；不得推导actual Provider |
| COP2-R39 | post-binding把ProviderTransportBinding复制为ProviderBinding | exact identity/Capability/currentness拒绝；零Entry、零Provider |

## 3. Prepared、Policy与Evidence闭包

| ID | 用例 | 预期 |
|---|---|---|
| COP2-C01 | Prepared current完整返回Delegation/Prepared/Persisted Enforcement | 与Request stable snapshot逐字段exact |
| COP2-C02 | caller Checked time不同但stable语义相同 | 不要求Checked逐字相等；Gateway持久fresh Projection |
| COP2-C03 | Prepared ID/revision/digest/Delegation/Enforcement/Provider/Payload漂移 | 零Entry、零Provider |
| COP2-C04 | Evidence Policy与Applicability Policy交换/合并 | 拒绝 |
| COP2-C05 | Applicability Profile漂移 | 拒绝；Profile不能来自request默认值 |
| COP2-C06 | Handoff、Qualification、Scope断链或phase/Attempt互换 | 拒绝 |
| COP2-C07 | Effect current kind/revision/digest与Provider Capability不一致 | 拒绝 |
| COP2-C08 | Boundary rollback、execute Enforcement/Handoff漂移 | actual-point读取拒绝 |

## 4. TTL与跨Owner窗口

| ID | 用例 | 预期 |
|---|---|---|
| COP2-T01 | Unified NotAfter | 等于Intent、Evidence/Applicability Policy、RouteCurrent、七个Binding、Prepared、Boundary、Enforcement、Handoff/Qualification和caller deadline的最小值 |
| COP2-T01a | 七个Binding逐项成为最短TTL | 每一项分别截断NotAfter；不得写成六个或遗漏ProviderTransport/Provider之一 |
| COP2-T02 | fresh clock为零、回拨或恰好到NotAfter | Entry前拒绝 |
| COP2-T03 | current read后Owner撤销、Provider admission前漂移 | 记录已读水位；不宣称窗口消失，Provider按stable key+NotAfter admission |
| COP2-T04 | Provider方法入口已进入但NotAfter过期 | Provider原子admission返回可验证no-effect receipt或unknown；不承诺方法零进入 |
| COP2-T05 | raw receipt无法证明未admit | Entry收`unknown`，不得伪造Observation |
| COP2-T06 | 未冻结Owner/Reader的上游policy expiry被加入NotAfter | 合同/Conformance拒绝；不得虚构expiry |
| COP2-T07 | 未来Operation Policy存在 | 仅作exact current门禁；无冻结TTL Owner时不参与NotAfter |
| COP2-T08 | Provider返回可验证未admit receipt | Entry终态`rejected_no_effect`、无Observation、同stable key不可重Execute |
| COP2-T09 | no-effect receipt缺失/伪造/无法验证 | Entry只能unknown；不得标记`rejected_no_effect` |

## 5. create claim、并发与lost reply

| ID | 用例 | 预期 |
|---|---|---|
| COP2-L01 | 首次create | `disposition=created`且仅本call stack获得opaque claim；Runner一次 |
| COP2-L02 | existing exact Entry | `disposition=existing`、无claim、只Inspect、Runner 0 |
| COP2-L03 | 64并发同stable key | 一个created/claim/Runner；63个existing或Inspect |
| COP2-L04 | 同stable key changed content | 一个胜出，其余Conflict；不能换Entry ID |
| COP2-L05 | create回包丢失后detached Inspect看到entered | 无claim；本次Runner 0；只返回entered/unknown |
| COP2-L06 | opaque claim复制、持久化、序列化或跨call stack | 不可表达/Conformance拒绝；Runner 0 |
| COP2-L07 | Authorization进入Result、日志、sidecar或Tool返回 | schema/conformance失败 |
| COP2-L08 | Authorization重放或跨Attempt使用 | opaque claim失效；Runner 0 |
| COP2-L09 | Provider reply lost | Entry unknown；只Inspect原Prepared/Attempt，不重调Runner |
| COP2-L10 | observed/unknown CAS lost reply | exact Inspect恢复；Runner不增加 |

## 6. Inspect、Runner与Owner越权

| ID | 用例 | 预期 |
|---|---|---|
| COP2-B01 | Inspect caller换Prepared或Attempt | Port不接受替换坐标；只从Entry读取原stable key |
| COP2-B02 | raw Provider return直接填Observation | 拒绝；必须经Provider State Plane Inspect得到exact Observation |
| COP2-B03 | raw Provider绕过Runner | assembly/import/conformance拒绝；Provider计数0 |
| COP2-B04 | Tool Adapter直接持raw Provider或Entry FactPort | conformance拒绝 |
| COP2-B05 | Runner推进Evidence Consumption | Runner无写Port；调用计数0 |
| COP2-B06 | Runner推进DomainResult/Settlement/Continuation | Runner无写Port；调用计数0 |
| COP2-B07 | unknown后再次Execute而非Inspect | 拒绝；Provider入口不增加 |
| COP2-B08 | Result冒充Outcome/DomainResult/Settlement | type/version拒绝 |
| COP2-B09 | `rejected_no_effect`携Observation或缺receipt | Result/Fact Validate拒绝 |

## 7. V1兼容与停止线

| ID | 用例 | 预期 |
|---|---|---|
| COP2-V01 | Harness active-route scan报告V1与V2同时active | Runtime fresh复读后Fail Closed；零Entry、零Provider |
| COP2-V02 | V1 fixture声称production eligible | 必须false |
| COP2-V03 | V1成功/Boundary/Observation升级V2 | 拒绝；无sidecar升权 |
| COP2-V04 | V2 Result推进后续Owner事实 | 调用数0；本切片停在Result/Inspect |
| COP2-V05 | production root/backend/SLA claim | Conformance必须false |

## 8. 实际实现门禁

已覆盖unit、whitebox、blackbox、fault、64并发、lost reply、immutable/progressed recovery、七Binding与entered-time closure、恶意Store重Seal、Route public shape/AST冻结。实际通过：

```text
go test -count=1 ./tests/ports ./tests/fakes -run ControlledOperationProvider
go test -count=100 ./tests/ports ./tests/fakes -run ControlledOperationProvider
go test -race -count=20 ./tests/ports ./tests/fakes -run ControlledOperationProvider
go test -count=1 -shuffle=on ./...
go test -race -count=1 -shuffle=on ./...
go vet ./...
gofmt -l <本纵切相关Go文件>        # 无输出
git diff --check -- .              # PASS
```

第三轮独立代码审计最终为`YES`，P0/P1/P2均为0；本矩阵不构成production backend、SLA或物理exactly-once证明。
