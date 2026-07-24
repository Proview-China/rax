# Runtime Operation Settlement Current Reader V5

状态：**第二次独立代码短审YES（P0/P1/P2=0/0/0），本纵切完成**。

## 作用

`OperationSettlementCurrentReaderV5`向Sandbox Checkpoint等消费者提供单方法current读取能力，使其无需持有包含`SettleCheckpointPhaseV5`的Governance写面。Runtime Settlement Owner、V5四对象闭包和shared terminal guard保持不变。

## 组成

- `runtime/ports/operation_checkpoint_settlement_v5.go`：additive单方法Reader及Governance兼容嵌入；
- `runtime/kernel/operation_checkpoint_settlement_gateway_v5.go`：request Operation/Effect与returned Bundle exact交叉；
- `runtime/conformance/operation_settlement_current_reader_v5.go`：reader-only public Conformance；
- `runtime/tests/ports/operation_settlement_current_reader_v5_test.go`：签名、method-set、capability narrowing与import边界；
- `runtime/tests/fakes/operation_settlement_current_reader_v5_test.go`：Gateway注入、typed-nil与恶意backend零泄露反例。

## 不变量

- Reader恰好一个方法；Governance仍为原六方法；Fact Port不变；
- Gateway先Validate完整Inspection，再以`SameOperationSubjectV3`和双EffectID exact比较request；
- drift返回零Inspection+Conflict，不泄露其他Operation/Tenant closure；
- consumer只注入Kernel Gateway facade提供的`OperationSettlementCurrentReaderProviderV5`，不注入raw Fact Port；
- current Inspect不授Settle、Provider、Evidence、DomainResult、ApplySettlement、Consistency或Restore权力。

## 验证

- target ordinary `count=100`：PASS（ports 0.007s，fakes 12.715s）；
- target race `count=20`：PASS（ports 1.032s，fakes 29.776s）；
- Runtime full ordinary shuffle：PASS（tests/fakes 5.099s）；
- Runtime full race：PASS；
- `go vet ./...`、gofmt、diff-check：PASS。

本模块已通过第二次独立代码短审；仍不声明production backend、root、durability或SLA。
