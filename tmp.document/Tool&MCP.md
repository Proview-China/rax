## Tool&MCP Documentary

### 综述

- Tool&MCP这一部分负责把异构能力整理成模型可以发现,系统可以治理,执行点可以验证,开发者可以复用的Action Plane.
- 它看起来像是注册几个函数和启动几个MCP Server,但是只要Tool能够写文件,访问网络,调用数据库,发送邮件或者产生费用,它就已经进入真实Effect边界.
- Tool和MCP需要分开理解.Tool是Praxis内部的能力语义和执行合同;MCP是一种发现,连接和调用外部能力的标准协议.MCP Tool可以映射成Praxis Tool Capability,但是Tool Engine不能被MCP协议完全定义.
- Tool Engine拥有Tool Descriptor,版本,Schema,注册,调用,结果规范化,取消,并发和工具级Effect Receipt;MCP Gateway拥有Server连接,传输,会话,能力快照,协议兼容和生命周期.
- Harness负责把Model Tool Call变成Action Candidate并推进交互循环;Review负责判断;Sandbox负责实际执行边界;Runtime负责Operation Governance,Permit,Fence和最终Settlement.工具回包不能自己成为Runtime Outcome.
- 我们还需要承认不同模型家族的Tool Calling行为不一样.GPT,Codex,Claude,Gemini,Qwen,Kimi和其他模型会偏好不同的编辑原语,参数形状,并行方式和工具说明.
- 所以Praxis应该提供统一Semantic Capability,再通过Model Profile和Harness Capability Profile将它翻译成不同模型能够稳定使用的Tool Dialect.这种适配不能改变真实权限和Effect语义.
- Skill,MCP Server,App Server,WASM Component和工具说明可以组合成Tool Package,形成后续生态和市场的基础;但是文本Skill本身不是执行授权,安装Package也不等于允许其中所有能力运行.

### 关于Tool和MCP的边界

- Tool Descriptor描述一个稳定能力,包括名称,版本,输入输出Schema,Effect Class,Scope,并发,超时,取消,幂等,Review Profile,Sandbox Requirement和Evidence要求.
- MCP Gateway负责发现MCP Server暴露的Tools,Resources和Prompts,形成带版本和来源的Capability Snapshot,再由Tool Engine决定哪些能力可以映射进入当前Agent的Tool Surface.
- MCP连接成功只表示Server可达,不表示其中的Tool已经获得授权;Tool出现在Catalog也不表示它应该进入模型Context;Tool进入模型Context更不表示可以自动执行.
- Tool Engine不能拥有MCP Server的连接和传输细节;MCP Gateway不能绕过Tool Engine和Runtime直接执行外部Effect.
- MCP Resource通常是可读取的Context Source,它可以被Context Engine引用或者按需展开;MCP Tool是Action Capability;MCP Prompt是外部提供的提示资产.三者必须保持类型区别.
- 标准MCP兼容层必须尽量忠实,扩展能力通过显式Capability Negotiation和独立命名空间提供,不能修改标准字段以后仍宣称完全兼容.

### 关于Model Profile和Tool Dialect

- GPT在大范围代码改动中可能偏好Shell或者Patch,在局部修改中偏好apply_patch;Claude通常偏好Read,Edit和Write;其他模型可能只稳定支持Function Calling或者Chat Completions式Tool Call.
- 这种差异应该进入Model Behavior Profile,Harness Capability Profile和Tool Dialect Adapter,而不是让每一个业务Tool分别针对每家模型重写.
- 比如上层语义都可以叫`workspace.edit`,Codex Route映射为apply_patch,Claude Route映射为Edit,某个Direct API Route映射为调用方托管的edit function.它们产生的Mechanism Trace不同,但是Effect Subject和Workspace Change语义保持一致.
- Model-visible tools和allowed/pre-approved tools必须分开.前者表示模型能够看见什么,后者表示哪些候选可能无需额外Human Approval;最终能否执行仍然需要当前Identity,Authority,Scope,Review,Budget,SandboxLease和Fence.
- Tool名称,顺序,Schema和说明会影响模型行为与Prompt Cache,所以Tool Surface必须稳定排序,版本化并进入Expected/Actual Injection Manifest.
- 不能为了提高模型调用成功率而向模型展示一个实际上无法执行的Tool,也不能隐藏额外内置工具然后宣称Tool Surface精确可控.任何Residual都必须在Profile和Conformance中明确.
- Tool Dialect Adapter只做表达转译,不能修改Effect Class,扩大文件或网络范围,降低Review要求,替换Credential Ref或者把有副作用Tool伪装成只读Tool.

### 关于核心对象

- `CapabilityDescriptor`描述能力语义,Owner,版本,输入输出,Effect Class和治理要求.
- `ToolDescriptor`描述具体Tool实现,Provider,Artifact Digest,Schema,协议,并发,超时,取消,幂等和Conformance.
- `ToolSurfaceManifest`描述本次Agent真正向模型公开的Tool集合,顺序,名称,Schema Digest,Guidance和Dialect.
- `MCPServerDescriptor`描述Server身份,来源,版本,传输,认证,信任级别和能力边界.
- `MCPConnectionRef`描述一个已经建立但仍受治理的连接或Session,不能把Provider Session ID当成Praxis权威身份.
- `CapabilitySnapshot`记录某个MCP Server或者Tool Provider在特定Revision实际暴露的能力.能力漂移必须生成新Snapshot并触发Reconcile.
- `ActionCandidate`绑定Model Call,Tool,输入Payload,Revision,Digest,Scope,Effect Class和期望Owner;它还不是执行授权.
- `ToolExecutionObservation`和`ToolResult`记录Provider回包,输出,执行状态和Evidence;它们需要Inspect和Settlement以后才能形成稳定Effect结论.
- `ToolPackageManifest`组合Skill,Tool,MCP,App Server,WASM,权限需求,签名,来源和兼容性,用于安装和装配,不能替代运行时授权.

### 关于MCP Server生命周期

- MCP Gateway至少需要支持Register,Resolve,Connect,Initialize,Discover,Bind,Health,Refresh,Call,Cancel,Inspect,Drain和Close.
- Server来源可以是本地进程,远程HTTP,Container,Sidecar,受管服务或者其他Transport.公共合同不应该把某一种Transport写进核心语义.
- 每个连接需要绑定Tenant,Identity,Agent Plan,Instance,Credential Ref,Network Policy,Server Descriptor和Capability Snapshot,不能在不同租户之间无声复用认证状态.
- Server返回的Tool List,Resource List和Prompt List都属于外部Observation.网关必须做Schema,数量,大小,名称,版本和安全检查,再形成内部Snapshot.
- 动态能力刷新不能在Run中无声改变模型Tool Surface.新增,删除或修改Tool需要新的Surface Revision,Context Generation或者明确Degradation Policy.
- 断连,重启和Session Resume需要新的Connection Epoch.旧Epoch迟到回包只能进入历史,不能污染当前调用.
- 恶意或者异常Server可能返回Prompt Injection,超大Schema,伪造成功,重复响应和不一致结果.所有外部内容按不可信数据处理,不能成为新的系统指令.

### 关于MCP Plus,Skill和Tool Package

- MCP Plus参考项目继续保留为扩展设计输入:[Praxis-Agent-Architecture/MCP-Plus](https://github.com/Praxis-Agent-Architecture/MCP-Plus).它提供的能力卡,折叠和组合经验需要经过当前Tool&MCP合同重新验证,不能直接等同于本模块实现.
- 我们可以吸收MCP Plus中对能力卡,工具折叠,按需展开,Skill+MCP+App组合和高并发任务的思路,但是扩展层要建立在标准MCP兼容面之外.
- Skill是带有索引,说明,适用条件和工作流程的Context Asset.它可以帮助模型正确使用Tool,但是不能因为Skill文本写着“允许执行”就获得Capability Grant.
- 一个Tool Package可以包含Skill说明,Tool Descriptor,MCP Server配置,App Server适配器,WASM Component,测试,示例,签名和供应链Evidence.
- Package安装分为获取,校验,注册和启用.获取到本地不等于注册成功,注册成功不等于进入某个Agent Plan,进入Plan也不等于运行时自动授权.
- Package必须声明Namespace,Version,Publisher,Artifact Digest,Dependencies,Supported Model Profiles,Requested Capabilities,Effect Classes,Sandbox Requirements,Review Profiles和Uninstall/Cleanup方式.
- 本地开发包可以使用开发签名和显式不可信状态;进入团队或市场以后需要来源验证,签名,透明日志,漏洞撤回,版本冻结和可重复构建证据.

### 关于Action调用链

- 统一调用链应该是:Model Tool Call进入Harness以后先形成Action Candidate,经过Schema和Capability Admission,再进入Review Gate和Runtime Operation Governance,获得精确Permit以后才能进入真实Tool或者MCP Provider.
- 执行前,Tool Gateway和Sandbox Enforcement Point需要再次读取当前Identity,Authority,Scope,Action Revision,Payload Digest,Review Verdict,Budget,Capability Grant,Credential Grant,Lease和Fence.
- Provider执行以后返回Observation或者Receipt,Tool Engine负责规范化结果和错误,MCP Gateway负责协议状态,Runtime与领域Owner通过Inspect和Settlement形成当前Effect结论.
- 结果再通过Context Engine成为ToolResult Fragment,进入下一轮Context Frame.Harness负责循环推进,但是不能修改Tool Result的来源和执行状态.
- Bypass Review也必须形成明确Verdict或者Operation Not Required记录,不能因为Profile是YOLO就跳过全部治理和Evidence.
- 只读Tool也需要Scope,超时,大小和数据泄露边界.没有写操作不代表没有敏感信息暴露,费用消耗或者Prompt Injection风险.

```text
Model Tool Call
  -> Action Candidate
  -> Capability/Schema Admission
  -> Review Gate
  -> Runtime Permit + Sandbox Enforcement
  -> Tool/MCP Execute
  -> Provider Observation + Inspect
  -> Tool Result + Effect Settlement
  -> Context Fragment
```

### 关于并发,流式,取消和Unknown

- 并行Tool Call需要为每个Action分配独立ActionID,Revision,Idempotency Key,Attempt和结果位置,不能依赖返回顺序与请求顺序永远一致.
- 对同一文件,同一数据库记录或者同一Effect Domain的冲突操作需要序列化或者使用明确的并发控制;只读且独立的调用可以并行.
- 流式结果要区分Progress,Partial Output,Final Result和Terminal Error.部分内容可以用于UI观察,但在Final之前不能被当成完整Tool Result.
- Cancel首先形成Cancel Intent并传播到Provider.Provider确认取消不等于外部Effect没有发生,仍然需要Inspect实际状态和残留.
- 网络断开或者回包丢失以后,如果Provider可能已经执行,状态必须进入Outcome Unknown.恢复路径只能Inspect原Attempt,不能创建一个新Attempt盲目重试.
- 只有被Tool Descriptor明确声明为安全幂等,并且当前Permit仍然有效的调用,才可以按照Policy重试;幂等声明本身也需要测试和Evidence.
- Batch完成以后需要产生Batch Result Manifest,明确每个Action的成功,失败,拒绝,取消,Skipped,Synthetic或者Unknown状态,不能用一个总成功掩盖部分失败.
- Tool Result,Schema和大型二进制内容需要大小上限,Artifact化和背压.不能把无限输出直接灌入Context或者Event Store.

### 关于Review,Sandbox和安全边界

- 每个Capability至少声明Effect Class,Risk Class,Default Review Profile,Required Authority,Filesystem Scope,Network Scope,Secret Scope,Budget和Sandbox Isolation Requirement.
- Review形成绑定精确Action Revision和Payload Digest的Verdict;Target变化以后旧Verdict失效.工具不能通过改写参数表面形式规避Review结论.
- Sandbox负责实际文件,网络,进程,Secret和执行后端门禁;Tool Engine不能因为自己验证过Schema就声称行为已经安全执行.
- Secret只通过Credential Ref或者Brokered Capability进入实际执行点,不能进入Tool Schema,Context,日志,Package Manifest或者MCP外部内容.
- WASM可以作为Tool Package的一种执行载体,适合低权限,高并发和能力清晰的工具;它不是所有Tool都必须使用的后端,也不能替代Runtime和Sandbox的当前事实检查.
- Provider Tool,MCP Server,Skill文本和Tool Result全部是不可信输入.它们可以提供数据和Evidence,不能修改系统指令,权限,Review Policy或者Runtime State.

### 关于Registry,市场和版本治理

- Registry负责保存Descriptor,Version,Artifact Digest,Publisher,Capability,Compatibility,Conformance,Deprecation和Revocation状态.
- Tool Alias只能在装配阶段解析成精确版本和Digest,Run中不能因为Alias指向变化而更换实现.
- Capability Snapshot,Tool Surface Manifest和Package Manifest都需要版本,摘要和来源链,并进入ResolvedAgentPlan和Harness Bootstrap检查.
- 升级需要比较Schema,Effect Class,权限,Sandbox要求和行为兼容性.一个看似Patch版本的更新如果扩大网络范围或者改变副作用,就必须视为不兼容升级.
- 撤回和漏洞修复需要支持阻止新装配,标记活跃Instance,触发Reconcile以及保留历史可验证性,不能直接删除旧版本导致Timeline无法解释.
- 市场可以提供评分,测试和签名,但不能成为最终Authority.组织Policy和用户Grant决定某个Package能否被实际Agent使用.

### 关于Tool&MCP SDK,CLI和API

- SDK需要提供Register Tool,Register MCP Server,Resolve Capability,Compile Tool Surface,Connect,Discover,Call,Cancel,Inspect,Watch Result,Drain和Close等能力.
- 核心类型至少包括CapabilityDescriptor,ToolDescriptor,ToolSurfaceManifest,MCPServerDescriptor,MCPConnectionRef,CapabilitySnapshot,ActionCandidate,ToolExecutionObservation,ToolResult,ToolPackageManifest和PackageEvidence.
- CLI可以提供`praxis tool list`,`praxis tool inspect`,`praxis tool call`,`praxis mcp status`,`praxis mcp discover`,`praxis package verify`和`praxis package install`等入口.
- API需要支持流式结果,幂等键,CAS Revision,分页,能力快照,权限过滤,取消,Inspect,Webhook和长任务状态.
- SDK和CLI发起的调用也必须走同一条Action,Review,Runtime和Sandbox治理链,不能把开发者接口变成后门.

### 和其他组件的联动

- 和Harness联动时,Harness消费Tool Surface并把Model Tool Call变成Action Candidate;Tool&MCP返回受治理的结果和等待状态,不控制Run终态.
- 和Context&Cache联动时,Tool Definition,Skill,Capability Card,Tool Call和Tool Result成为类型化Context Fragment;Context负责顺序,展开,预算和缓存.
- 和Review联动时,Tool提供Risk,Effect和Target材料,Review形成Verdict;Tool不能自己决定Human,Auto或者Bypass.
- 和Sandbox联动时,Tool&MCP声明执行需求,Sandbox选择和强制执行文件,网络,进程,Secret和隔离边界.
- 和Continuity联动时,Capability Snapshot,Connection Epoch,Action Candidate,Attempt,Progress,Result,Settlement和Residual进入Timeline.
- 和Memory&Knowledge联动时,Tool可以读取候选内容或者产生Memory/Knowledge Candidate,但是不能直接把结果写成正式记忆和知识.
- 和Runtime联动时,Runtime拥有Operation Intent,Permit,Fence,Execution Governance和Settlement;Tool&MCP拥有能力语义,协议和领域结果.

### 我们最后要落成的

- 一套Tool Engine和MCP Gateway分离但协作的Action Plane,既保持Praxis能力语义,又兼容官方MCP标准.
- 一套Provider无关的Capability,Tool,Surface,Server,Connection和Snapshot合同.
- 一套Model Profile与Tool Dialect映射,让不同模型使用适合自己的原语,同时保持统一Effect语义和数据记录.
- 一条Action Candidate,Admission,Review,Permit,Enforcement,Execute,Inspect,Result和Settlement的完整调用链.
- 一套并发,流式,取消,幂等,背压,Batch和Outcome Unknown处理机制.
- 一套Skill+Tool+MCP+App Server+WASM的Tool Package和供应链治理机制,为后续生态和市场提供基础.
- 一套Registry,版本冻结,兼容检查,签名,撤回和漏洞响应能力.
- 一套Tool&MCP SDK,CLI和API,让开发者可以注册,发现,编译,调用,检查和扩展能力,又不能绕过统一治理.
- 一套与Harness,Context,Review,Sandbox,Continuity,Memory&Knowledge和Runtime稳定接轨的Versioned Port和Event合同.
- 最重要的是,我们最后产出的不是另外一个内置几十个工具的Coding Agent,而是一套能够管理工具生态,适配不同模型,控制真实Effect并让开发者持续扩展的能力基座.
