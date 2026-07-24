# Harness Controlled Operation Provider Route V2吸收第五审并进入第六短审

## 事件

Runtime公共Route V2合同通过第三轮独立审计后，Harness Owner完成Declaration、确定性多Declaration merge、结构化Conflict、Conformance、Route Current CAS Store/Reader Adapter与完整静态no-bypass编译合同；现已吸收Harness第五审最后三项P2问题并进入第六短审。

## 已闭合

- 直接复用Runtime六个中立Route公共类型，无复制、alias或反向import；
- required Manifest extension间接进入既有AssemblyInput摘要，不改V1字段或canonical；
- deterministic CurrentID、单调CAS、lost reply exact Inspect、same-ID drift Conflict；
- ConformanceRef、Generation/Handoff/BindingSet/ActiveRoute、七Binding完整Watermark闭包；
- ProviderTransport与actual Provider分离；Candidates、Ports、Slots、Factories、Dependencies、Hook/Phase、V1/V2 active route与sealed wiring双层no-bypass；
- 64并发单一publish、TTL crossing、clock rollback、七Binding漂移和import/type Owner反例。
- `ControlledOperationProviderRouteConformanceInputsReaderV2`与OwnerSource均通过assemblyadapter包内未导出方法封闭；OwnerSource只返回stable exact refs，verified Compile、ActiveRoute、WiringInventory及Runtime Association/七Binding由独立Reader复读；
- Declaration/Wiring/Conformance三个Seal拒绝错误非零digest；Builder/Current Reader拒绝typed-nil依赖；
- 当前Harness基线为188个Test、7个Fuzz入口（共195个），按`go test -coverpkg=./... -coverprofile=<temp> ./...`汇总实测跨包语句覆盖率75.1%。

## 第三审修复

- Conformance不再接受调用方完整Snapshot；只接exact lookup key，并从Owner Reader读取verified compile result、registered Governance Catalog digest、完整Assembly artifacts/current facts后复算；
- Declaration/Conformance同ID改为immutable create-once；Current保留单调CAS并拒绝历史Watermark回流与`A→B→A`；
- sealed wiring由末两段扩大为ApplicationPort→ToolAdapter→RuntimeGovernancePort→Gateway→Transport→Provider完整五段；
- no-bypass加入Candidate/Module/ComponentManifest/Artifact/Capability/Port conflict domain结构身份，拒绝alias组合与extra edge；
- 新增64个不同内容并发一胜、自签Inline/空Publisher、ABA/immutable与完整链缺边反例。

## 第四审修复

- 从真实Manifest/Module/PortSpec/Candidate重建ProviderTransport与Provider post identity，完整绑定PortSpecDigest与ConflictDomain；post-binding Conflict Side增加TransportBinding/ProviderBinding；
- 所有Candidate/Port/Slot/Factory/Dependency/Hook/Phase alias统一产sealed `provider_alias_conflict`，可由`errors.As`取得并重算digest；
- V1 RouteBinding只经Harness-local exact active-route sidecar恢复normalized identities/Bindings，再产sealed `active_route_version_conflict`；缺sidecar Fail Closed；
- 新增Harness-local exact Owner Artifact Store及Compile/ActiveRoute/Wiring独立Reader；外包stub不能经公开OwnerSource/FromOwner绕过sealed InputsReader；
- 状态等待Harness Route V2第六短审。

## 第五审修复

- Conflict新增AssemblyInput provenance与closed Phase；prebinding alias/V1只携AssemblyInput，postbinding active-route精确携AssemblyInput/Graph/Wiring；
- 新增closed AliasSurface，Candidate/Port/Slot/Factory/Dependency/Phase分别按Kind强制唯一Ref/Module/Port/Capability形状与canonical digest；Dependency以Ref/PortSpecRef精确绑定From/To双端，完整source由Conflict的AssemblyInputDigest绑定；
- Legacy Fact实现closed `active|inactive|revoked`状态并绑定sealed WiringInventory proof；active产生Conflict，inactive/revoked只有exact absence proof成立时放行，缺失或漂移Fail Closed。

## 未扩大

没有修改Runtime、Tool、Application或Context；没有生产Backend、root、Capability启用、Provider调用、Settlement、Continuation或Turn推进。

## 验证

最终高重复、Race、全量、Vet、格式与diff门禁结果记录在本任务最终回传中；本事件不把内存Store声明为生产实现。
