# Governed Model Turn V3 Actual Boundary

## 冻结目标

本切片只把已存在的 `GovernedModelTurnV3` 与 `InvocationMaterialV2` 串进 routegateway 的可逆 pre-invoke 路径。由于 credential exact current、实际 Provider request canonical 和 durable Provider Attempt 尚未冻结，它在 `prepareAt` 后、任何 Boundary 写入前确定性 Fail Closed。

## 固定顺序

```text
exact Turn
→ historical Boundary early Inspect
→ authoritative Context/Tool pair S1
→ Prepared/Current/ACK S1/S2
→ reversible Provider prepare
→ deterministic Fail Closed
```

历史 Boundary 通过同一复合 Store 在 Binding、Secret 和 Factory 前 early Inspect；所有新调用都保持 Boundary、Provider 与 Observation 写入为零。

## 边界

- `ContextFrame/ContextMaterial`与`ToolInjectionMaterial/ToolSurface`必须通过`InvocationMaterialAuthorizerV2`的paired-reader ports再次验证。
- caller传入的非零`ModelBoundary`必须与历史winner exact一致；只有零值可由历史winner补全。
- S1/S2分别用一次权威paired-reader读取同时生成Authorization与完整Context/Tool projection；每条source的Ref、Checked、TTL、Digest及projection body必须exact一致，不能二次读取拼接快照，也不能只比较聚合最短TTL。
- Commit ACK在S1/S2都必须重新验证current、Prepared/Current绑定和完整body；expiry equality、same-Ref body splice及重封后的Ref漂移均在Provider prepare前拒绝。
- exact Turn Reader返回值必须同时通过完整`Validate`和Ref equality，不能用保持Ref的State/Created/Updated/Expires splice绕过。
- routegateway不生成Context、Tool、Runtime或Harness事实。
- Provider prepare可逆；确定性 Fail Closed 必须释放adapter lease。
- adapter lease的release/retire终态与调用顺序无关；最终stale且refs为零时Closer恰执行一次，release错误保留在返回因果链。
- 当前切片不跨 Runtime guard、不调用 Provider，也不产生 Provider Observation。
- 同版本、同expiry但credential material generation变化不能由现有`SecretResolver`证明，因此不得通过二次Resolve解锁Provider。
- direct `Invoke`、V1、V2路径不因本切片获得V3治理语义。

## Production NO-GO

当前同时缺少：

- 将`prepareAt`产生的实际Provider request形状重算并闭合到`ActualProviderInjectionDigest`的公开canonical；
- V3 Provider Attempt/Observation的持久化与exact Inspect合同。
- Model-owned exact `CredentialMaterialCurrentRef`、`ResolvedCredentialHandle`与`CurrentReader`，并让pool key和`preparedCall`绑定该exact ref，在invoke前fresh exact Inspect。

所以本切片只能证明pre-invoke读取、准备与Fail Closed；测试中的工具alias和compiled digest仍是fixture材料，不是Tool Owner authoritative compiled material。Host availability只能由Runtime guard的权威reader提供，caller不得伪造。
