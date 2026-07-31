# Governed Model Turn V3 Actual Boundary Plan

## 实施

1. 增加已持久化`InvocationMaterialV2`的authoritative pair复读helper；Authorizer每个阶段只读每个Pair一次，并从同次读取同时生成Authorization与完整current projection。
2. 为Gateway增加独立V3 dependencies与Option，不修改V1/V2。
3. 实现S1/S2与可逆Provider prepare。
4. 在任何Boundary写入前因credential exact current合同缺失而确定性Fail Closed。
5. 在任何Provider prepare前校验Runtime actual-point draft；历史replay拒绝caller Boundary splice。
6. 增加64并发、same-version/same-expiry material drift、非min source TTL/Checked漂移、ACK current/body漂移、Turn body splice、pair unavailable、clock drift和replay测试。
7. 收紧adapter lease终态：release/retire并发或任意先后顺序不丢失Closer与错误。

## 验收

- Boundary、Provider与Observation写入均为0。
- 64并发同Turn/attempt保持Boundary=0、Provider=0。
- 同版本同expiry但material generation变化仍Provider=0。
- 任一authoritative source或ACK在S1/S2发生完整projection/body漂移时，Binding、Secret、Factory、Boundary、Provider与Observation均为0。
- 交替Reader不能利用同阶段二次读取制造Authorization/Pair混合快照；每个Pair在每个阶段恰读一次。
- exact Turn保持Ref但修改State/Created/Updated/Expires时，Provider prepare为0。
- authoritative pair未配置时Gateway无法启用。
- V1/V2、direct Invoke、Provider实现、Runtime/Harness/Context/Tool目录零修改。
- ordinary、race、vet、diff/import boundary通过。

## 后续

另行冻结Provider request canonical→`ActualProviderInjectionDigest`闭包、V3 Provider Attempt/Observation history与lost-reply exact Inspect，以及credential exact current/handle/reader；本计划不生成production Outcome。
