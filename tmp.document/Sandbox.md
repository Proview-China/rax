## Sandbox Documentary

### 综述

- 这一部分其实很容易被写成一个Docker管理器,或者是一个Firecracker管理器.
- 但是本质上,我们想要构建的并不是某一种容器,而是一套可以让agent安全获得执行环境,并且控制它如何接触真实世界的运行边界.
- 所以我先把几个概念分开:agent是带有identity,状态和记忆的逻辑实体;Sandbox是执行权和隔离边界;Docker,MicroVM,裸金属和远程机器只是不同的执行后端;WASM则更像是能力和工具的安全载体.
- 也就是说,开发集群,法务集群,财务集群和销售集群并不是四个大容器.它们本质上是四个逻辑上的namespace和policy domain.
- 前置语义层已经通过投影将知识送到了正确的位置,那么Sandbox要做的事情,就是继续守住这些位置:谁可以在哪运行,可以看到什么,可以调用什么,以及最后能把什么行为真正提交到外部世界.
- 我们不能将agent和进程绑定起来.上千个agent identity不代表上千个常驻container,更不代表上千个常驻MicroVM.
- 但是一个已经进入执行状态的AgentInstance,应该获得一份独占的SandboxLease.这个Lease可以绑定到共享worker pool中的一个隔离槽位,也可以绑定到独立container,MicroVM,裸金属workspace或者远程sandbox.
- 所以我们最终要做的是一个Sandbox Abstraction Layer,让上层只关心隔离需求,权限范围和任务风险,而不是提前绑定某一种后端产品.

### 关于Sandbox公共对象和权威

- `ExecutionRequirement`描述任务需要的操作系统,架构,工具链,文件,网络,进程,Secret,资源,持久化,隔离级别和Checkpoint能力,它是装配需求,不是已经获得的权限.
- `SandboxPolicy`把Identity,Authority,Scope,Capability Grant,Review,Budget和组织策略编译成当前Instance可以执行的边界,并绑定Revision,Digest和TTL.
- `BackendDescriptor`描述Host,Container,MicroVM或者Remote Provider真正能够强制执行什么,包括Isolation,Conformance,Fence,Inspect,Cleanup,Checkpoint和Residual能力.
- `PlacementDecision`记录为什么选择某个Backend和执行槽位,对应什么Requirement与Policy,以及允许什么显式降级.它属于调度和装配证据,不等于SandboxLease.
- `SandboxLease`是AgentInstance获得执行权的唯一当前凭证,至少绑定Instance,Epoch,Backend,Policy,Workspace,Capability Grant,Lease TTL,Fence Epoch和当前状态.
- `WorkspaceView`描述Agent实际可以看见的文件树,只读挂载,可写Overlay,临时目录,Artifact Ref和隐藏区域;它不能只是一组未经验证的宿主路径.
- `WorkspaceChangeSet`记录Base Revision,实际Change,Diff,Artifact Revision,Effect,提交状态和Evidence,用于Review,Continuity和选择性Rewind.
- `EffectEnforcementRequest`是实际执行点的第二次门禁输入;`ProviderObservation`和`ExecutionReceipt`是后端回包;`CleanupReport`和`ResidualReport`描述环境回收结果.
- Provider回包只证明Provider声称完成了某件事.Sandbox Controller必须通过独立Inspect,Scope/Epoch/Digest校验和CAS形成当前Lease,ChangeSet和Cleanup事实.
- Runtime拥有Instance和Operation Governance,Sandbox拥有Lease与执行环境事实,Scheduler拥有资源放置策略,Backend拥有具体执行机制.这四个Owner不能被一个Sandbox Manager类全部吞并.

### 关于宿主机上的工作

- 我不认为agent在宿主机工作的时候应该直接抛弃沙箱.
- 因为本地coding agent最大的价值,恰恰就是直接读取真实项目,使用真实工具链,并把结果留在用户真实的workspace中.
- 但是如果我们为了安全把整个workspace都封死,agent就失去了本地工作的意义;如果我们直接把宿主机的shell和文件系统全部开放,那么一次错误判断就可能伤害用户资产.
- 所以宿主机模式需要的不是一台很重的MicroVM,而是一层更轻的执行治理:受控的command executor,范围化的文件视图,Effect Gateway,review_gate,审计记录和可恢复的diff.
- agent看到的应该是一个被投影出来的workspace view.只读资产,可写资产,临时目录,secret和网络范围都应该被单独声明.
- 对文件的修改不应该立刻等于对宿主机事实的修改.更好的方式是overlay+diff+staging:agent先在自己的写层工作,系统记录行为和变化,最后再通过策略或者人工审核提交到真实workspace.
- 但是这里也要诚实:只要我们把不受控制的bash直接交给agent,它就可能绕过文件映射和能力接口.所以宿主机模式必须收紧原始执行路径,或者明确降级安全等级,不能一边开放任意shell,一边宣称所有行为都已经被Sandbox控制.
- 这也是为什么Sandbox不只是一个权限表.它还需要知道实际执行点在哪里,并在行为真正发生之前再次检查当前的identity,scope,lease,fence和review状态.

### 关于集群和执行后端

- 在集群环境中,我们要先区分业务集群和计算集群.
- 开发,法务,财务和销售是业务域.每个域拥有自己的知识视图,权限策略,工具包,审核方式和数据边界.
- 真正承载计算的地方则应该是runtime worker pool.大量agent identity被scheduler按任务分配到不同的执行面,而不是一个agent永久占据一个container.
- 普通的检索,文档处理,受控工具调用和可信代码,可以进入共享的container worker或者更加轻量的执行槽位.
- 需要完整Linux工具链,但是风险仍然可控的任务,可以使用独立Docker container,并按照部署环境继续叠加gVisor,Kata之类的隔离能力.
- 不可信代码,高风险外部输入,强多租户,敏感数据和需要独立内核边界的长任务,更适合进入Firecracker一类的MicroVM.
- 但是Firecracker也不应该成为唯一答案.它依赖Linux/KVM,而Praxis本质上要做IaaS式的框架,所以我们应该定义隔离等级和后端合同,再让Docker,MicroVM,裸金属和远程Provider分别证明自己能满足什么.
- 因此,不是每个agent一个Firecracker,也不是每个部门一个Docker.更加合理的是共享的container pool,按需的MicroVM pool,以及被Lease临时占用的执行槽位.
- AgentInstance结束或者被fence之后,执行面必须进入独立的inspect和cleanup.只有文件,进程,网络,secret和远端残留都得到确认,这个槽位才可以安全复用.

### 关于WASM

- 我们在这里必须把WASM的位置说清楚.
- WASM不是完整Linux环境,所以没有必要强行把coding agent,浏览器agent或者需要大量系统工具的agent整体塞进WASM.
- 但是WASM非常适合装载一个边界清晰的tool,skill,policy module或者数据转换能力.它可以默认没有文件,网络和进程权限,只获得宿主显式授予的capability.
- 所以不是agent的每一个动作都对应一个新的WASM,而是每一个需要跨越保护边界的动作,都必须经过统一的Capability Gateway;其中某些能力可以由可复用的WASM Component实现.
- 比如filesystem.read,workspace.write,http.request,send_email和database.query,都可以拥有稳定的能力合同.一个WASM模块可以服务大量agent和大量调用,没有必要为每次动作重新制造一份字节码.
- WASM也不能单独成为最终的权限权威.真正的allow,deny,require_review还需要读取SandboxLease,identity,scope,authority,budget和review verdict等当前事实,并且留下可以审计的evidence.
- 如果某个agent已经在MicroVM里面运行,WASM仍然可以负责隔离第三方skill和细粒度tool;但是如果任务只是调用一个纯函数式工具,我们也没有必要为它再启动一台MicroVM.
- 这就形成了一个比较舒服的分工:WASM负责能力级隔离,container负责常规工作环境,MicroVM负责独立内核级隔离,Sandbox Controller负责选择,绑定和治理它们.

### 关于文件映射,行为提交和恢复

- 我比较认可将agent到真实host的行为再慢一步,但是这个“慢一步”不应该只是性能上的延迟,而应该是一套明确的事务边界.
- 文件读取可以通过受限view直接发生;文件写入先进入overlay;最终提交时生成diff,绑定原始版本,agent,instance,run和effect,再经过策略或review进入真实文件系统.
- 这种方式天然可以和Continuity联动.Continuity记录timeline,event,checkpoint和可恢复历史;Sandbox记录某次执行实际接触了哪些文件,产生了什么diff,以及这些diff是否已经提交.
- 但是网络请求,邮件发送,交易和远程数据库写入不能像文件一样假装可以回滚.这些外部Effect必须先记录intent,再dispatch,最后用receipt和inspect完成结算;结果不确定时不能盲目重试.
- Restore也不是把世界倒回去.它应该从一个一致的checkpoint创建新的AgentInstance,使用更高epoch和新的SandboxLease,并把无法回滚的外部Effect和残留明确带到新的执行状态中.
- 这样我们才能获得真正有用的安全性:不是简单的“不让agent出去”,而是让每一个重要行为可限制,可观察,可审核,可结算,可恢复,并且知道哪些东西永远无法恢复.

### 关于状态,故障和Unknown

- SandboxLease至少经历requested,allocating,allocated,activating,active,fencing,fenced,releasing,released和indeterminate等状态,每次变化都需要Expected Revision和CAS.
- 同一AgentInstance在任意时刻只能拥有一份非终态Exclusive Lease.第二份Lease请求必须冲突失败,不能依靠Scheduler约定避免双重执行.
- Allocate,Activate,Fence,Checkpoint,Restore,Release和Cleanup都可能出现丢回包.一旦Provider可能已经执行,后续只能Inspect精确Operation和Provider Ref,不能重新派发一个新的Attempt.
- Fence只能缩减执行权.它可以使用Emergency Safety路径,但是不能借此创建Lease,恢复任务,扩大文件或网络权限.
- Cleanup Complete需要独立检查进程,文件挂载,网络连接,Secret路径,后台任务,远端执行和Provider保留状态.执行结束不等于环境已经可以复用.
- 残留被确认时进入Residual;覆盖不足或者状态未知时进入Cleanup Indeterminate.两者都需要占用冲突Effect Domain,直到新的受治理Cleanup或者Compensation收敛.
- Backend不可用时,Runtime可以选择等待,迁移或者使用Plan显式允许的降级Backend;不能因为容量紧张而静默降低Isolation和Conformance.

### 和其他组件的联动

- 和Harness联动时,Harness通过`sandbox.execution` Slot和受治理Port请求执行,等待结果和Cleanup Observation;它不能持有Backend原始句柄,修改Lease或者把Provider回包升级成Runtime事实.
- 和Context&Cache联动时,Sandbox提供Workspace View,文件Revision,进程,网络,Diff和Residual等Context Candidate;Context决定模型看见哪些观察,不能通过Context反向扩大Sandbox权限.
- 和Tool&MCP联动时,每个工具包都要声明capability,安全栅栏级别,review策略,网络和文件范围.其中skill+MCP+appserver可以组成工具包,WASM则可以成为其中一种安全封装方式.
- 和Review联动时,Sandbox负责在执行前强制门禁,Review负责产生绑定精确对象和版本的verdict.审核通过不等于已经执行,最后的dispatch仍然要由对应Gateway完成.
- 和Continuity联动时,Sandbox要提供checkpoint参与,workspace diff,执行残留和cleanup证据,但是不应该自己拥有timeline和恢复裁决.
- 和Memory&Knowledge联动时,候选内容可以在Sandbox中产生,但是进入正式记忆和知识库仍然属于一次受治理的Effect,不能因为agent写进了本地文件就自动成为正式事实.
- 和Runtime联动时,Runtime管理AgentInstance,identity,epoch和运行事实,Sandbox管理Lease,placement,isolation,fence,residual和cleanup.两边需要清晰分工,不能让Provider的一次成功回包直接成为系统权威事实.

### 关于Sandbox SDK,CLI和API

- SDK需要提供Describe Backend,Match Requirement,Plan Placement,Allocate,Activate,Inspect,Fence,Create Workspace View,Prepare ChangeSet,Commit ChangeSet,Checkpoint Participant,Release和Inspect Cleanup等能力.
- 核心类型至少包括ExecutionRequirement,SandboxPolicy,BackendDescriptor,PlacementDecision,SandboxLease,WorkspaceView,WorkspaceChangeSet,EffectEnforcementRequest,ProviderObservation,ExecutionReceipt,CleanupReport和ResidualReport.
- CLI可以提供`praxis sandbox backends`,`praxis sandbox inspect`,`praxis sandbox fence`,`praxis sandbox diff`,`praxis sandbox commit`,`praxis sandbox residuals`和`praxis sandbox cleanup`等入口.
- API需要支持异步Operation,幂等键,CAS Revision,Watch,取消,Inspect,Lease TTL,Checkpoint Barrier,Artifact Ref和Cleanup长任务.
- SDK,CLI和API只能通过Runtime与Sandbox Controller公共Port提交意图,不能直接调用Backend获得未经治理的执行权.
- Backend开发者可以实现新的Provider Adapter和Conformance Probe,但是必须通过同一套黑盒测试证明Fence,Scope,Inspect,Cleanup和Unknown恢复语义.

### 我们最后要落成的

- 一个Sandbox Controller,负责Lease,placement,隔离等级,后端选择,fence和cleanup.
- 一套ExecutionRequirement,SandboxPolicy,BackendDescriptor,PlacementDecision,SandboxLease,WorkspaceView,WorkspaceChangeSet和CleanupReport公共对象.
- 一套稳定的Sandbox Backend Interface,允许接入host,Docker,MicroVM和remote provider,而不是在内核中写死Firecracker.
- 一个Data Plane Enforcement Agent,在实际文件,网络,进程,secret和tool行为发生前完成第二次门禁.
- 一套Capability Gateway和可选的WASM Runtime,承载高并发,低权限,可验证的tool和skill.
- 一套workspace view+overlay+diff+commit的文件映射系统,并和Continuity的event,checkpoint和restore关系打通.
- 一套Evidence,Residual和Cleanup机制,确保执行完成不等于环境已经安全回收.
- 一套Sandbox SDK,CLI和API以及Backend Conformance测试,让开发者可以增加Provider而不能绕过Runtime和执行点门禁.
- 最重要的是,我们最后产出的不是一个新的Docker UI,也不是一个Firecracker封装器,而是一套能够让不同agent,不同业务域和不同风险等级共同运行的执行治理基座.
