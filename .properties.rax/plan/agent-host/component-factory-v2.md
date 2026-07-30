# ComponentFactoryV2 落地计划

1. 定义 additive V2 Descriptor、Attempt、Start request、Instance、Conformance、Registry Key、Registration 与 Preflight Request/Receipt。
2. 定义 `ComponentFactoryV2`、最小 Handle、Conformance/Dependency current readers、Registry 与 Preflight ports。
3. Start AttemptID由完整canonical Start payload派生，父RequestDigest与Attempt exact digest形成双层闭包；Schema MediaType使用canonical lowercase ASCII grammar。
4. 实现 sealed in-memory Registry；只保证typed-nil、Descriptor/Conformance exact、字段闭集与sealed resolve无漂移，注册阶段只读取descriptor metadata，不调用Factory，不识别实现来源。
5. 实现 Preflight：Deployment current → Builder完整Selection/Closure → closure manifest唯一Factory descriptor → Registry → Conformance → Resources/Dependencies，S1/S2 后封存权威current与确定性 `max(request/current checked)` Receipt。
6. 覆盖Start digest替换/同Attempt异payload/restart-lost-reply、MediaType非法UTF-8/uppercase/space/control/alias。
7. 覆盖Deployment/Selection/Closure/Registry/Conformance/Resource/Dependency逐轴S1/S2 drift，以及非空Resource+Dependency的Checked max、Expires min与expiry equality。
8. 覆盖64并发同payload同digest、同Attempt异payload Conflict；明确无durable Store与restart-safe仍为NO-GO。
9. 覆盖Descriptor/Conformance的reference-only、raw-provider、non-owner独立反例；诚实证明本地test factory可凭自报production形metadata结构注册，但`ProductionEligible=false`且不得production composition。
10. 运行 focused ordinary/race、全模块 ordinary/race/vet 与 diff-check；通过后进入独立终审。

产物仍是 owner-local admission/control合同，不验证实现provenance，不是 production Host composition；未来Build/Release Trust Owner不在本PR施工。
