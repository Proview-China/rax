# Controlled Operation Provider Route V2测试矩阵

## 1. 当前门禁

本文件所列Route测试已通过Harness Route V2第八独立短审`YES(P0/P1/P2=0)`；该结论不表示G6A fixture或production root已经实现。cross-module fixture仍等待system G6A验收，production root继续等待G6B完整验收与真实接线Conformance。

## 2. 合同与canonical

| ID | 用例 | 预期 |
|---|---|---|
| COPR2-U01 | 同Declaration重排可排序集合 | canonical digest稳定 |
| COPR2-U02 | nil/empty、重复项、unknown field、重复JSON key、尾随文档 | 严格拒绝或按字段规则唯一归一，不产生半对象 |
| COPR2-U03 | Port/Endpoint/Reader nominal ref互换 | type-pun拒绝 |
| COPR2-U04 | matrix不是唯一G6A tuple | Fail Closed |
| COPR2-U05 | required extension缺失、optional、未注册、Schema或payload digest漂移 | Compile前拒绝 |
| COPR2-U06 | 用`RouteBindings []ObjectRefV1`替代Declaration | 不满足required route，拒绝 |
| COPR2-U07 | Declaration复制到Runtime/Tool类型后重seal | 拒绝；Harness Declaration是唯一canonical定义，外域只消费exact Ref/current Projection |
| COPR2-U08 | ProviderInspect Reader缺失、与Boundary互换或带Execute能力 | Entry前拒绝；read-only/no-execute必须exact |
| COPR2-U09 | CurrentRef、DeclarationRef、ConformanceRef或flattened ActiveRoute坐标互换/重封 | nominal type-pun拒绝；Reader零返回、Runtime零Entry |
| COPR2-U10 | ProviderTransportBinding与ProviderBinding互换，或Prepared/Request只绑定前者 | 拒绝；Transport adapter binding不能替代实际Provider current proof |
| COPR2-U11 | CurrentRef缺字段、增加字段、Digest包含ProjectionDigest/自身Digest，或遗漏Watermark | canonical/Validate拒绝；Ref Digest只覆盖除Digest外六字段，ProjectionDigest独立 |
| COPR2-U12 | Harness重新定义/alias六个中立类型中任一项 | import/compile-time Conformance拒绝；DeclarationRef、ConformanceRef、CurrentRef、Projection、Reader、MatrixKeyV3唯一代码Owner为`ExecutionRuntime/runtime/ports` |
| COPR2-U13 | Adapter未满足真实Go签名，或用`any`/本地Matrix替代`OperationScopeEvidenceApplicabilityMatrixKeyV3` | 编译期接口断言失败；不得靠runtime转换通过 |
| COPR2-U14 | DeclarationRef/ConformanceRef缺字段、Revision为零、digest非法、增加自由map或外层ContractVersion/ObjectKind字段 | Runtime ports `Validate`拒绝；版本/kind只由canonical domain/type discriminator固定 |
| COPR2-U15 | Harness Declaration/Conformance Fact使用本地Ref struct或alias，或Runtime ports定义Harness事实canonical/digest | import DAG/Conformance拒绝；`type Owner != fact semantic Owner` |
| COPR2-U16 | Declaration缺独立`provider` role/candidate，或Provider与ProviderTransport互换/折叠/漏入digest | strict Validate/canonical拒绝；两层nominal声明必须独立 |
| COPR2-U17 | Current Ref/Projection字段类型或snake_case JSON tag与冻结Go struct不同 | 编译/canonical golden拒绝；不得保留旧三类未定型Ref占位 |
| COPR2-U18 | 七个角色复用Capability，或Reader role无法由field position + unique Capability + Conformance PortSpec映射唯一证明 | Conformance/Current零publish；不得用`ProviderBindingRefV2`字段互换冒充角色 |
| COPR2-U19 | 调用方伪造Compile/Generation/Handoff/BindingSet/currentness，或用`Inline='x'`、空Publisher Manifest自签Conformance输入 | Builder只接受exact lookup key并从Owner Reader复读；verified compile/registered Catalog/Assembly artifacts交叉验证失败，零写 |
| COPR2-U20 | 外包试图实现Conformance InputsReader或OwnerSource并经`FromOwner`构造Builder，或Builder/Current Reader注入typed-nil Store/Reader | InputsReader与OwnerSource的包内sealing method令外包不可实现；OwnerSource只返回stable exact refs；typed-nil构造期Fail Closed且零读取/零发布 |
| COPR2-U21 | Declaration/Wiring/Conformance调用Seal时携错误非零digest | `ErrorPreconditionFailed + ReasonInvalidDigest`；不得清空后重算洗白 |

## 3. merge、conflict与Binding

| ID | 用例 | 预期 |
|---|---|---|
| COPR2-M01 | 同Publisher/Route/revision/digest重复 | 幂等折叠为一项 |
| COPR2-M02 | 同matrix换任一Route/Port/Reader/Endpoint/Manifest/Artifact/Capability/Candidate | Conflict，无可执行Graph |
| COPR2-M03 | 两个active Gateway、ProviderTransport或actual Provider Binding | `one_active_binding`拒绝 |
| COPR2-M04 | 同一Transport或actual Provider以不同Candidate ID/Module/Port别名出现，并被任一PortSpec/Slot/Factory/Dependency/Hook/Phase/Route引用 | 分别按两组规范化身份元组识别alias，`no_raw_provider_bypass`拒绝 |
| COPR2-M05 | Generation/Handoff/BindingSet/ActiveRouteCurrent/Conformance任一digest或revision漂移 | Route Conformance rejected/expired |
| COPR2-M06 | Binding TTL跨界或clock rollback | 不返回current conformant，不授执行 |
| COPR2-M07 | V1与V2同时active且指向相同或别名ProviderTransport/actual Provider | 结构化跨版本Conflict；不把exact内容视为幂等 |
| COPR2-M08 | V1/V2同matrix分别指向两个Transport或Provider，或V2试图shadow V1 | 结构化Conflict；无priority/newer-wins，零current Projection |
| COPR2-M09 | test/production wiring inventory额外注入raw Transport或actual Provider句柄 | Conformance拒绝；Manifest Graph无旁路不足以放行 |
| COPR2-M10 | current Reader返回七个Binding current中任一错误值，或ProviderBinding与Transport Binding混淆 | Runtime Request/Gateway/Prepared exact比较拒绝，零Entry |
| COPR2-M11 | Reader按DeclarationRef或历史Conformance读取，而非exact CurrentRef | 接口/canonical拒绝；只能真实签名`InspectCurrentControlledOperationProviderRouteV2(context.Context, ControlledOperationProviderRouteCurrentRefV2, OperationScopeEvidenceApplicabilityMatrixKeyV3) (ControlledOperationProviderRouteCurrentProjectionV2, error)` |
| COPR2-M12 | 同CurrentID但Revision、Digest、Declaration/Conformance、Matrix或Watermark任一不同 | 结构化Conflict，零Projection/Entry；不得伪装NotFound |
| COPR2-M13 | CurrentID在Owner中根本不存在 | 返回NotFound；不得把same-ID mismatch降级成NotFound |
| COPR2-M14 | Projection重复字段与Ref的Declaration/Conformance/Matrix/Watermark不一致，随后重Seal ProjectionDigest | `Validate`逐字段拒绝；重Seal不能掩盖current事实漂移 |
| COPR2-M15 | malformed Ref、absent ID、same-ID drift、TTL crossing、Owner unavailable、真实性不可收敛 | 依次映射`core.ErrorInvalidArgument/ErrorNotFound/ErrorConflict/ErrorPreconditionFailed/ErrorUnavailable/ErrorIndeterminate`；用`core.HasCategory/HasReason`断言，无自由错误或跨类降级 |
| COPR2-M16 | Adapter新增`ErrExpired`、`ErrUnknown`或自定义Harness error enum | 编译/import Conformance拒绝；只允许`core.NewError`与既有Category/Reason |
| COPR2-M17 | 同DeclarationRef/ConformanceRef ID换revision、publisher、nested Declaration或digest | `core.ErrorConflict`；same canonical重放幂等，ID不存在才NotFound |
| COPR2-M18 | ConformanceID未按`Derive(RouteID, GenerationArtifactRefV1, BindingSetID)`生成，或Revision跳号/调用方覆盖 | Harness Conformance Fact Owner CAS拒绝，零Current publish |
| COPR2-M22 | Handoff或ActiveRoute同ID换revision/digest，或flattened字段缺失 | `core.ErrorConflict`/Invalid，零Current/Entry；不得发明新Ref类型掩盖漂移 |
| COPR2-M19 | CurrentID非`Derive(RouteID, MatrixDigest)`、Watermark遗漏任一current维度或Current在Conformance成功前发布 | Route Current Store零写拒绝；Current保持post-binding产物 |
| COPR2-M20 | Conformance/Current publish回包丢失、same canonical重复publish、64并发不同内容CAS、旧Ref读取 | 原Ref Inspect恢复；重复幂等；一个Revision线性化其余Conflict；旧Ref遇新Revision Conflict，revoked/expired Fail Closed |
| COPR2-M21 | Tool Adapter或Runtime Gateway按RouteID List/ResolveLatest/discovery，或自行构造CurrentRef | 接口/装配拒绝；composition只注入exact CurrentRef，Reader exact-key读取 |
| COPR2-M23 | Wiring inventory只含Gateway→Transport→Provider，遗漏ApplicationPort→ToolAdapter→RuntimeGovernancePort→Gateway | 完整五段链比较拒绝；后两段正确不能冒充系统闭合 |
| COPR2-M24 | Candidate换ProviderRef但复用Module/Component/Artifact/Capability/Port conflict domain，或新增extra/merge edge | 结构化归一身份识别alias并Conflict；不能只按ProviderRef或Module+Port二元组放行 |
| COPR2-M25 | Declaration/Conformance同ID换内容，或Current按A→B→A重用旧Watermark/Conformance | immutable create-once/monotonic CAS拒绝；exact replay仍幂等，64个不同内容只一胜 |
| COPR2-M26 | effect-free Port复用protected exact PortSpec、Phase复用alias Module或仅换Handler ID保留digest | 结构化归一matcher识别并Conflict；Dependency/Slot/Factory/Phase共享同一Module/Component/Port/conflict-domain语义 |
| COPR2-M27 | verified Compile result中的ProviderTransport/Provider `PortSpecDigest`、`ConflictDomain`或post-binding `TransportBinding/ProviderBinding`任一漂移 | 从真实Manifest/Module/PortSpec/Candidate重建expected identity并拒绝；零Conformance/Current publish |
| COPR2-M28 | Candidate、Port、Slot、Factory、Dependency、Hook/Phase任一路径形成Provider alias | 全部返回sealed `provider_alias_conflict`；携closed exact AliasSurface及其canonical digest；`errors.As`得到`ControlledOperationProviderRouteConflictErrorV2`且Conflict digest可重算 |
| COPR2-M29 | V1 `RouteBinding`存在但Harness Owner-current Reader缺失/不可用，Fact直传，RouteBindingRef与Record不相关，或V1/V2同时active | 缺Reader或exact current复读时Fail Closed；Owner Reader恢复左右normalized identity及V1 Binding后返回sealed `active_route_version_conflict`，不伪造V2 pre-binding Binding |
| COPR2-M30 | prebinding Conflict携Graph/Wiring digest，postbinding Conflict缺AssemblyInput/Graph/Wiring任一digest，或code/phase互换 | `Validate`拒绝；prebinding alias/V1只绑定AssemblyInput，postbinding active-route绑定三项provenance |
| COPR2-M31 | AliasSurface Kind开放，或Candidate/Port/Slot/Factory/Dependency/Phase的Ref/Module/Port/Capability closed形状错位，Dependency未绑定From/To双端，或CanonicalDigest漂移 | Seal/Validate拒绝；完整source由AssemblyInputDigest绑定，AliasSurface只允许该Kind的唯一定位形状；非Candidate surface不得回退protected identity |
| COPR2-M32 | Legacy state为active/inactive/revoked以外值，state与Active flag漂移，sealed wiring proof缺失/换digest/换record/过期；`now<checked`；或相同Fact在`[T0,T0+60s)`内依次以`T0+50s/T0+10s`完成S1/S2 | Fail Closed；要求fresh clock、`checked<=now<expires`、`nowS2>=nowS1`且S1/S2 Fact完全一致；跨读取回拨返回`ReasonClockRegression`，active产生version Conflict |
| COPR2-M33 | inactive/revoked proof未覆盖目标binding，或同matrix/Transport/Provider identity或Binding别名下另有active V1 route | Fail Closed；目标exact non-active record必须在sealed wiring中，扫描全部active V1，non-active不能掩盖其他active |
| COPR2-M34 | PortSpec以受保护ProviderTransport/Provider Capability形成alias | 生产路径产出closed `AliasSurface{Kind=port,Ref==PortSpecRef,Capability}`并返回sealed `provider_alias_conflict` |

## 4. G6A fixture六类硬反例

| ID | 注入点 | 必须行为 |
|---|---|---|
| COPR2-F01 | Entry create已提交但成功回包丢失 | detached Inspect只恢复exact Entry；丢失token的call frame `providerCalls=0`，Application/Tool进入inspect-only |
| COPR2-F02 | current读取后、Entry create前跨过Unified NotAfter | 零Entry、零Provider；不得换ID/内容重派 |
| COPR2-F03 | Entry成功、Provider Adapter方法已进入，但不可逆admission前跨TTL | 不承诺Adapter方法零进入；可验证`rejected_no_effect` receipt时`irreversibleAdmissions=0`、无Observation、同stable key只Inspect且不重执行；receipt缺失/丢失/不可验证则Entry `unknown` |
| COPR2-F04 | Provider已进入但响应/Observation outcome unknown | Entry与Tool Watermark永久inspect-only；Application保持`waiting_inspect`，后续不增加Provider次数 |
| COPR2-F05 | 64并发同canonical Request；另组同Route ID换内容 | 同canonical一个Entry且Provider最多1；换内容只有一个胜出，其余Conflict |
| COPR2-F06 | Application、Tool协调层、Slot、Factory、Dependency、Hook/Phase、其他Route或真实root直接取得raw ProviderTransport/actual Provider | 双身份全图/真实注入Conformance拒绝；旁路不可逆admission数0 |

## 5. 正向fixture与Owner边界

| ID | 用例 | 预期 |
|---|---|---|
| COPR2-I01 | test-only手工注入Application Tool Port、Runtime Governance Port、Gateway、ProviderTransport、其私有actual Provider接线及三类current Reader | Route Conformance/current Projection exact；Application/Tool协调层不取得Transport/Provider raw句柄，不注册Capability、不触碰production Session |
| COPR2-I02 | exact首次调用 | Application只见Tool Port；Tool Adapter只持Governance Port；Runtime私有路径唯一调用Transport，Transport只访问exact ProviderBinding |
| COPR2-I03 | Provider Observation返回 | 不自动推进Evidence Consumption、Tool DomainResult、Runtime Settlement或Harness Continuation |
| COPR2-I04 | Harness Assembly导入Tool/Runtime实现，或Application持ProviderTransport/Entry FactPort | import/Conformance拒绝 |
| COPR2-I05 | Runtime Request携历史Route proof但current Reader unavailable/漂移/过期 | 零Entry、零Transport；历史Conformance不能替代exact CurrentRef对应的current Projection |

## 6. production enablement NO-GO

Runtime V2、Tool Adapter、本Route P1-P3、G6A、G6B及production no-bypass Conformance任一未完成时，production root保持NO-GO。Fake只证明合同、故障与并发，不证明真实Backend、SLA或production root。
