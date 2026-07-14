# Runtime Binding Manifest V2 设计落地说明

## 1. 目的

Binding Manifest V2是Runtime面对官方、第三方和用户自定义组件的确定化治理入口。它只描述组件如何被识别、探测、认证和绑定，不解释组件领域Payload，也不授予Effect Dispatch资格。

本切面保留现有`v1alpha1`组件合同，新增独立的`praxis.runtime.binding/v2`合同，避免通过扩展旧Observation改变其权威等级。

## 2. 公共边界

- `runtime/core`：domain-separated Canonical Digest、严格JSON解码、严格SemVer和机器可判定错误码。
- `runtime/ports`：Manifest V2、SchemaRef、Opaque Envelope、治理Catalog、注册/Probe面和v1受限Adapter。
- `runtime/control`：BindingFact、BindingSet、CAS状态机、稳定DAG解析及当前性检查。
- `runtime/conformance`：不写权威事实的Adapter Conformance检查。`CheckAdapterRuntimeImportsV2`是可调用的构建扫描门禁，只允许Adapter导入`runtime/core`与`runtime/ports`；通过扫描不授Binding、production或Dispatch资格。
- `runtime/fakes`：线程安全、原子CAS的测试Fact Store；不声明生产Conformance。

组件领域内核不得导入Runtime实现。只有组件的Runtime Adapter可以依赖`runtime/core`和`runtime/ports`；Adapter只能得到`context.Context`和版本化Envelope，得不到Runtime Kernel、Foundation、Fact Store或内部句柄。

## 3. 确定化规则

1. ID、Kind、Capability和治理类别统一使用小写ASCII `namespace/name`，拒绝Unicode、大小写漂移和平台Normalization差异。
2. SemVer严格解析；Build Metadata参与Binding身份摘要，但Range比较按SemVer忽略Build Metadata。
3. Canonical Digest包含domain、contract/version和type discriminator；不同合同即使JSON字节相同也不会共享身份摘要。
4. Manifest中的集合在摘要前按Canonical Key排序并拒绝重复；nil和empty只在这些明确的set字段中等价。
5. Effect、Settlement、Cleanup三个角色各有且仅有一个Owner；同一组件可以承担多个角色。
6. SchemaRef绑定namespace、name、version、media type和content digest。
7. Opaque Payload必须二选一使用有限Inline或Reference，并绑定Schema、Content Digest、长度及Limit Policy；Runtime不解析业务正文。
8. 治理Extension进入Binding Digest；展示Annotation不进入执行身份。未知optional Extension保留并可往返，未知required Extension拒绝。
9. 未声明顶层字段严格拒绝；前向兼容只能进入已定义Extension Envelope，防止旧Reader静默丢字段后重新签名。
10. `BindingPlanV2.PlanDigest`由完整Plan canonical派生，不再信任调用方自报。摘要排除自身字段，Requirements按ComponentID、RequiredCapabilities按namespaced key排序；语义相同的重排与nil/empty set保持同摘要，替换Kind、Artifact、Contract、Capability或required/residual语义必然漂移并拒绝。

## 4. 权威状态机

```text
declared
  -> probed
  -> certified
  -> bound
  -> expired | revoked
```

- Register返回不带时间的`ComponentRegistrationObservationV2`，只能证明注册表接纳；Probe返回带注入Clock时间的`ComponentProbeObservationV2`。两者都不产生Grant。
- `CapabilityDeclaration`不是`CapabilityGrant`。
- Grant只存在于经过CAS的BindingFact中，并绑定Manifest Digest、Artifact Digest、Contract、治理Catalog Digest、Evidence和TTL。
- BindingSet通过原子Commit把所有成员从certified推进到bound。
- TTL边界按`now < expires`判定；相等即过期。Clock必须注入，回拨至持久Probe时间之前时Fail Closed。
- Manifest、Artifact、Contract、Grant、依赖或治理Catalog漂移都会使旧Binding不可继续使用。
- Bound仍不代表Dispatch资格；Dispatch Permit由后续P0.2 Governance Gateway产生。

## 5. 自定义组件

Runtime不硬编码6+1组件Kind。自定义组件使用Namespaced Kind，并在Governance Catalog注册：

- 治理类别；
- 可提供Capability；
- Schema；
- 允许的Locality与Conformance；
- 可识别的治理Extension。

注册成功不等于认证成功，认证成功不等于绑定成功，绑定成功也不等于Dispatch资格。Conformance testkit在没有独立Certification Fact/attestation输入时最多返回“认证候选”，Binding、Production和Dispatch资格始终为false。声明外Capability、未知required Schema/Extension、Owner缺失、依赖循环和过期Grant全部Fail Closed。

## 6. 当前限制

- 本切面尚未提供生产Catalog/Binding持久后端、远程注册协议或进程拓扑。
- 当前性检查已冻结为公共Validate函数；持续Reconcile调度器将在后续Runtime协调阶段接线。
- P0.2之前没有DispatchPermit，因此任何Binding都不能据此直接执行Effect。
