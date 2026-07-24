# Sandbox Production Wiring端到端接线

时间：2026-07-18 00:12 CST  
状态：`production_wiring_implemented / software_and_live_backend_test_yes / deployment_pending`

## 粗粒度事件

用户解除普通Sandbox lifecycle上层接线限制并授权继续完成。Go控制面保留Owner分层，Rust继续
作为独立Data Plane；未解禁Checkpoint/Restore、Snapshot purge、持久State Plane产品选择或SLA。

## 已落地

- Application新增公开`SandboxLifecyclePortV4`与`SandboxLifecycleCoordinatorV4`；Application只见
  Plan exact ref、Operation/Effect/Attempt与Runtime DomainResult/Settlement exact refs，不导入Sandbox实现。
- Sandbox `applicationadapter`闭合prepare/execute独立Runtime V4 Enforcement、Evidence
  Issue/Handoff/Record/Consume、Rust Dispatch、Observation、独立governed Inspect、Sandbox
  DomainResult、Runtime Settlement与Sandbox ApplySettlement。
- 新增`NewProductionCompositionV4`，只组合宿主注入的Runtime公共Gateway、Sandbox持久Owner Store、
  Application Plan/Result Store、Go/Rust UDS和reverse-current Server；不构造Runtime实现，不接受Provider
  实现，不用testkit替代State Plane。
- 独立Inspect使用新的Effect/Attempt/Permit/双Enforcement，Evidence闭表为
  `activation_attempt + praxis.runtime/activation-inspection-evidence + praxis.sandbox/inspect`；target
  exact绑定原EffectKind、AttemptID、revision-2 ProviderAttempt、request digest与payload digest。
- Rust durable journal持久完整Provider Result；Dispatch回包丢失只用exact original request Inspect，
  Completed不重放Provider，Started保持Unknown。containerd Inspect按Lease/Scope/资源provenance复核；
  allocate attempt要求label exact，activate/open通过同Lease资源与payload digest关联。
- Runtime V4 Settlement closed matrix同步允许`praxis.sandbox/inspect`，与Evidence closed matrix一致。

## 关键不变量

```text
Application Request
  -> Runtime prepare Enforcement -> Evidence handoff -> Rust Prepare -> Evidence consume
  -> Runtime execute Enforcement -> Evidence handoff -> Rust Execute -> Evidence consume
  -> independent Inspect Operation/Attempt + same two-phase chain
  -> Inspect DomainResult -> Runtime Settlement -> Sandbox ApplySettlement
  -> original DomainResult -> Runtime Settlement -> Sandbox ApplySettlement
```

- Begin不授Provider执行；Rust实际执行点第三次复读Runtime V4与Sandbox exact current。
- Provider永远只产Observation/Receipt；不产Evidence、DomainResult、Settlement、Lease/Fence/Outcome。
- `Unavailable/Indeterminate`不当`NotFound`；lost reply只Inspect原identity；Unknown不递归创建Inspect链。
- Settlement lost reply必须Inspect到同submission digest，再复读同Effect current closure；另一Fact/Owner/
  DomainResult/revision全部拒绝。
- Checkpoint/Restore、Snapshot Artifact外部Retention/purge、Host/MicroVM/Remote、持久State Plane具体实现、
  Assembly、部署认证与SLA仍为NO-GO或pending。

## 实际验证

- Sandbox：`go test -count=100 -shuffle=on ./...` PASS；
  `go test -race -count=20 -shuffle=on ./...` PASS；full ordinary/race、`go vet ./...`、`gofmt -l` PASS。
- Application：coordinator定向ordinary100/race20 PASS；full ordinary/race/vet PASS。
- Runtime：Evidence/Settlement定向ordinary100/race20 PASS；full ordinary/race/vet PASS。
- Rust：`cargo test --all-targets`、`cargo test --release --all-targets`、strict clippy、fmt PASS；真实
  Wasmtime Component矩阵PASS；受控真实containerd
  `allocate -> activate -> independently governed inspect -> release/cleanup` PASS。
- containerd测试临时启动宿主service，结束后已恢复`inactive`。

## 后续边界

下一步不是继续扩张Provider，而是由宿主提供真实持久Fact/Plan/Result/DomainResult binding Store，
把Sandbox current reader注入Runtime Gateway，安装并监督Rust daemon，再执行进程级crash/partition/
credential rotation/升级回滚与SLA系统验收。未完成这些门前不得写“已生产部署”。
