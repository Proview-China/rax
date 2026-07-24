# Runtime V3授权签发与Association写口软件验收YES

时间：2026-07-18 01:40 +08:00

状态：`implementation_software_test_yes`，仅owner-local/reference-test。

本轮为受治理MCP `tools/call`补齐两处此前只有nominal、没有真实Owner入口的接线：

- Runtime additive `PreparedDomainCommandAssociationPortV1`按Prepared/Attempt/DomainCommand
  stable ID执行create-once Ensure与exact current Inspect；Runtime Owner负责Checked/Expires/
  Ref/Digest，调用方不能自签Projection，过期后不续命；
- Runtime additive `ControlledOperationPhysicalAuthorizationPortV3`复读V2完整current closure、
  Association与DomainCommand，在fresh S1/S2 clock下签发统一NotAfter授权；Gateway不调用Provider；
- reference in-memory Store覆盖lost create reply、same canonical 64并发单winner、Owner/TTL/
  typed-nil/nil-context/clock rollback和exact drift反例；
- Tool actual physical executor、MCP Protocol Receipt及Receipt→Evidence→正式Observation既有分层不变。

这不代表production GO。持久State Plane Association/Evidence Source、production composition root、
Credential、stdio/Streamable HTTP、远端输出策略、MCP专用DomainResult/Settlement与G6B仍未闭合。
