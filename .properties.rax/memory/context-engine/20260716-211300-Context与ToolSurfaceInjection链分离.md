# Context与Tool Surface Injection链分离

时间：2026-07-16 21:13（Asia/Shanghai）

状态：`corrected`。本轮只修Context design/plan/memory，没有修改Go、Harness、Tool或Model。

## 裁决

live核对确认两条链名称相近但完全独立：

1. Context链：`Context ExpectedInjectionManifest → ProviderActualInjectionObservation → Harness ActualInjectionManifest → Context InjectionConformanceFact`，用于核验Frame/field实际注入；
2. Tool Surface链：Tool Owner `ToolSurfaceManifest.ExpectedInjectionDigest → Tool Surface Gate`，用于Tool Surface pre-dispatch等式与门禁。

Context ExpectedInjectionManifest不是Tool Surface expected digest，Tool Surface Gate不消费Context Expected Manifest或InjectionConformanceFact。Context也不为满足Surface等式新增Expected Manifest exact-current Reader、Port、projection或隐式digest映射。

此前本任务短暂创建的`ExpectedInjectionManifestExactCurrentReaderV1`候选design/plan/memory已撤销，不作为P0/P1/P2阻塞或后续实现输入。

## 保留边界

Context Expected/Actual Injection Conformance仍按现有Owner分层独立演进；Tool Surface current Reader/Gate由Tool Owner负责。两链只能在更高层审计中关联各自exact facts，不能互相授予current、Evidence、Permit、Capability或执行资格。
