# ComponentFactoryV2 设计

## 目标

在 Agent Host 内建立通用的组件工厂声明合同、密封 Registry 与零副作用 Preflight。该切片只证明 Builder Package 声明、Host Descriptor、Conformance admission metadata 与 current 事实在结构和 exact 坐标上一致，不证明 Go 实现来源或 provenance，也不启动组件。

## 边界

- Builder 拥有 `AgentPackageSelectionCurrentV1`、`VerifiedAgentPackageClosureV1`、Package、Publication 与 `ModuleFactoryDescriptorV1`。
- 各 Component Owner 拥有 Factory 实现，并发布 Descriptor、Conformance Current、Instance、Inspect 与 Cleanup 声明。
- Agent Host 只拥有通用接口、Registry metadata 与 Preflight Receipt。
- Preflight 请求只绑定 H1 `HostDeploymentCurrentRefV2`、Factory Registry Key、资源/依赖 exact refs、Attempt 与时间窗；调用方不得声明 Package、Publication 或 Closure。
- Preflight AttemptID 由去除 AttemptID/RequestDigest 后的 canonical request 唯一派生；RequestDigest再绑定 AttemptID与该identity digest，因此同Attempt异payload无需Store即可Fail Closed。
- Preflight 通过 Builder 权威 reader 读取并在 Receipt 中封存完整 `AgentPackageSelectionCurrentV1`、`VerifiedAgentPackageClosureV1` 及其原生 PackageRef、PublicationRef、ClosureDigest 与 ModuleFactoryDescriptor。

## 安全约束

- Descriptor/Conformance 的 Implementation、ProviderAccess、ReferenceOnly、Trust 与 Evidence 只是 sealed declaration/admission metadata；Validate只拒绝明确标记的reference-only、raw-provider与non-owner声明，不验证实现来源。
- Registry 只承诺 typed-nil 拒绝、Descriptor/Conformance exact、字段闭集、sealed 后不可注册及 resolve 时无漂移；不通过reflect、包路径或字符串黑名单识别fixture/internal/testkit来源。
- 本切片没有 Build/Release Trust Owner 的实现 provenance 证明；所有 Registration 均封存 `ProductionEligible=false`，不得进入 production composition。
- Registry sealed 后不可注册；exact key包含 Factory、Conformance、Module、Artifact、Capability、Schema、Cleanup 与 Trust。
- Component Schema MediaType采用与Runtime SchemaRefV2等价的canonical lowercase ASCII `token/token` grammar：有效UTF-8、最多128 bytes、恰好一个`/`、slash两侧非空且只允许固定ASCII token字符；大写、空白、control、非法UTF-8与等价alias全部拒绝。
- Start AttemptID由去除自身ID/digest后的完整canonical Start request body唯一派生；父RequestDigest封存完整Attempt identity，Attempt Ref digest覆盖全部exact坐标。相同Attempt异payload确定性Conflict，Inspect只接受原exact Attempt Ref。
- Preflight 做完整 S1/S2 复读，TTL 是 Deployment、Selection、Resource、Dependency、Conformance 与请求上界的最小值。
- Receipt封存完整已验证Request preimage、Builder权威Selection与VerifiedClosure；Selection.Ref必须逐字段等于Deployment.PackageSelectionRef，Selection的Package/Publication/Closure必须逐字段等于VerifiedClosure。
- Receipt中的ModuleFactoryDescriptor必须由封存VerifiedClosure的Publication.Manifest按FactoryID唯一精确读取；caller同步替换descriptor与registration也不能重封。
- Receipt封存权威Deployment、Selection、Conformance、Resource与Dependency current；Checked epoch确定性取 `max(Request.RequestedUnixNano, all authoritative current.CheckedUnixNano)`，公开Validate从封存current重算，禁止声称在Owner事实产生前完成校验。
- S1/S2仍使用实时单调clock验证current，并要求 `checked <= sealNow < expires`；sealNow不写入Receipt，从而同Request/Attempt在事实未漂移时得到同一Receipt digest。
- Host/Start/Deployment/Registry/Resource/Dependency/Request window及Package/Publication/Closure逐轴闭合，清空外层digest也不能重封splice。
- Preflight 不持有 executable Factory，不调用文件、数据库、Socket、环境、网络、Provider 或 Effect。
- `ComponentHandleV2` 只暴露 Instance、InspectBinding、CleanupBinding；清理由既有 Host Cleanup Owner 链执行。

## 非目标

不注册真实 6+1 Factory，不接 HostV3 Start/Stop，不构造或清理资源，不产生 SystemReady，不实现未来 Build/Release Trust Owner，不声明 production。

## 已知风险

本切片没有durable preflight/Start Store，也没有实现来源/provenance验证；本地test factory可自报production形metadata并通过结构注册，但Registration仍为`ProductionEligible=false`。不声明进程外create-once、lost-reply resolution或restart-safe恢复；pure deterministic identity只保证一个完整canonical payload对应唯一Attempt/Receipt，production与跨重启语义保持NO-GO。
