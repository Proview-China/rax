# Port Delta：Execution关闭后的独立Cleanup Inspect

状态：提交Runtime Owner评审；Harness不直接修改`ExecutionRuntime/runtime/**`。

## 1. 用例

Harness `Close`可能只完成“拒绝新Control并发出取消”，远程进程、Provider Session、Sandbox资源或未知Effect仍在清理。Runtime必须能在Close回包后独立Inspect清理状态；当前Endpoint一旦标记closed，既有`ExecutionPort.Inspect`会被Harness Adapter拒绝，无法区分`closing`、`cleaned`、`unknown`和`residual`。

## 2. 语义Owner

- Harness/Execution Surface：拥有清理过程和Cleanup Observation；
- Runtime Kernel：拥有实例Cleanup聚合、失败真实性和最终CAS；
- Effect Settlement Owner：拥有未知外部Effect的Inspect/Settlement；
- Provider回包或签名Receipt不得自动成为Runtime权威事实。

## 3. 输入与输出

建议由Runtime Owner选择兼容形式之一：

1. 新增可选窄接口`ExecutionCleanupInspectPort`；或
2. 允许既有`Inspect`携带专用`cleanup` kind访问已关闭但仍可验证的Endpoint。

最小输入：Execution Scope、Endpoint Ref、Close Effect Intent ID+Revision、Cleanup Inspect Kind。

最小输出：Source ID/Epoch、状态`closing|cleaned|unknown|residual`、Receipt/Evidence Digest、ObservedAt、Residual Ref（若有）。输出仍是Observation。

## 4. 不变量

- Close后禁止Start/Input/Action/Cancel等一般Control，但允许只读Cleanup Inspect；
- Scope、Instance/Sandbox/Authority Epoch和Endpoint Digest必须匹配，旧Epoch只作迟到证据；
- `cleaned`不能仅由Close成功回包推导，必须可独立Inspect；
- Observation不得直接写成Runtime Cleanup事实；Runtime独立检查后CAS；
- Cleanup Inspect不得扩大Scope、恢复执行权或创建新Effect。

## 5. Effect与Recovery

- Close本身继续绑定持久化Intent和当前Fence；
- Close回包丢失或Cleanup为`unknown`时只能Inspect，禁止盲目重复Close；
- Inspect发现Residual时保留冲突域和Cleanup Owner，不能伪报释放完成；
- 若Inspect需要远程读取，读取动作必须遵守对应Effect/离线策略。

## 6. 反例

远程Model调用忽略取消。Harness `Close`返回`closed` Observation并立即Fence Endpoint；Runtime随后调用`Inspect(cleanup)`却收到`fenced_instance`。此时Runtime既不能证明进程已停，也不能证明Provider Session已清理，却可能被迫把“Endpoint不可访问”误写成Cleanup完成。

## 7. 兼容影响

直接给`ExecutionPort`增加方法会破坏所有实现，风险高。推荐新增可选窄接口，或在保持方法签名不变的情况下冻结`cleanup` Inspect语义并允许closed Endpoint只读访问。Runtime Owner需同时补Foundation故障注入、Fake和Conformance测试后再串行合入。
