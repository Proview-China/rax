# Runtime Model Dispatch Control Current Reader V1

本模块为Model Provider actual-point guard提供Runtime-owned cancel/fence/revoke/desired current投影。

代码：

- `runtime/control/model_dispatch_control_current_reader_v1.go`
- `runtime/conformance/model_dispatch_control_current_reader_v1.go`
- `runtime/tests/control/model_dispatch_control_current_reader_v1_test.go`
- `runtime/tests/conformance/model_dispatch_control_current_reader_v1_test.go`

公共/窄合同：

- `control.ModelDispatchRunFactCurrentReaderV1`
- `control.ModelDispatchCommandFactCurrentReaderV1`
- `control.NewModelDispatchControlCurrentReaderV1`
- `control.ModelDispatchControlCurrentReaderV1`

边界：

- 只读Run、Desired State、Command历史，不写Runtime事实；
- 不读取Context cancel，不复制Model/Harness状态；
- 返回projection最大TTL为1秒，actual-point guard仍需复读；
- 已通过owner-local普通、race、full、vet与格式门；
- 尚无生产SQLite Run/Command Owner或composition root，因此production NO-GO。
