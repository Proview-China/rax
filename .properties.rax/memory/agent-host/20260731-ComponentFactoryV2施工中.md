# 2026-07-31 ComponentFactoryV2 施工中

- 已从合并 H1 后的 `origin/main@b29d9542` 建立独立 worktree。
- 已建立 additive V2 contract、ports、sealed registry 与 zero-side-effect preflight。
- 早审后删除了 Host 自造的 Package/Publication/Closure DTO；Preflight Request不再携带 caller 声明的跨 Owner事实。
- Receipt封存 Builder完整 `AgentPackageSelectionCurrentV1`、`VerifiedAgentPackageClosureV1`、原生 PackageRef、PublicationRef、ClosureDigest与Package ModuleFactoryDescriptor。
- Preflight已采用完整canonical Request的纯函数Attempt身份闭包；同Attempt异Host/Start/Deployment/resource/dependency/time/registry在Owner reader前Conflict，Receipt RequestDigest不可splice。
- Receipt Validate从封存closure的Publication.Manifest唯一重取Factory descriptor；Selection的Package/Publication/Closure及descriptor artifact/module/capability/schema/cleanup/trust逐轴替换均不能公开重封。
- Receipt封存Deployment/Selection/Conformance/Resource/Dependency权威current，Checked epoch确定性取Request与全部current checked的最大值；Owner current晚于request的正例及提前篡改反例已覆盖。
- 同Request/Attempt在clock推进时保持同digest；64并发同payload同digest，异payload全部Conflict。
- Start AttemptID已改为完整canonical Start payload的稳定派生值；父RequestDigest封存完整Attempt identity，Attempt Ref digest覆盖全部坐标，digest替换/复用ID/异payload/restart-lost-reply篡改均Fail Closed。
- Component Schema MediaType已采用Runtime等价canonical lowercase ASCII grammar，非法UTF-8、uppercase、space/control与alias反例已覆盖。
- GO裁决采用收窄声明：Implementation/ProviderAccess/ReferenceOnly/Trust/Evidence仅是sealed declaration/admission metadata；Registry不识别fixture/internal/testkit实现来源，所有Registration均为`ProductionEligible=false`，未来Build/Release Trust Owner保持切片外。
- 非空Resource+Dependency正例已证明Checked=max全部current、Expires=min全部current；Deployment/Selection/Closure/Registry/Conformance/Resource/Dependency逐轴S1/S2 drift与now==expiry均Fail Closed且Factory/effect调用为0。
- 当前只允许 Owner-local 合并，不注册真实6+1 Factory，不接HostV3，不声明production。
- 当前没有durable preflight/Start Store，不声明进程外create-once、lost-reply resolution或restart-safe恢复；production与跨重启语义保持NO-GO。
