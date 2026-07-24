# SurfaceInvocationBinding P4-1软件验收YES

## 事件

- Tool Owner在`ExecutionRuntime/tool-mcp`内完成P4-1：`ToolSurfaceInvocationBinding*V1` public Writer/Reader/Repository、Binding/Ack canonical合同、Tool Owner唯一create-once内存Repository与测试fixture；
- 唯一Repository在一个线性化边界原子维护Binding ID与Invocation coordinate双索引，same canonical返回同一winner，same Invocation换canonical返回Conflict；
- Owner clock将Prepared Historical NotAfter、Prepared Current、Tool Surface Current、Runtime Assembly Current、required RequestedNotAfter与caller deadline压为共同NotAfter，clock rollback或TTL crossing零写；
- Ensure回包丢失只按Invocation Inspect既有Binding/Ack；历史Fact过期后仍可exact Inspect，但`ValidateCurrent` Fail Closed；
- Binding/Ack canonical、Model neutral SurfaceBindingRef无损映射、deep clone、nil/canceled context、typed-nil、canonical drift及64并发单winner反例已闭合；
- 独立软件验收确认targeted ordinary/race、full ordinary/race、`go vet ./...`、并发、lost-reply、typed-nil与hash稳定全部PASS。

## 当前门

- P4-1状态：`implementation_software_test_yes`；该YES仅代表Tool Owner SurfaceInvocationBinding纯软件实现与测试验收；
- Binding/Ack不授Provider执行权，不代表Model/Harness/Runtime/Tool四Owner完整actual-point接线或production GO；
- P4-2+的InputContract、ActionCandidateV3、BindingV2、Harness M2 wiring、每个attempt/Open/Stream/continuation四读、system、production root/backend与能力启用继续锁定；
- 本事件仅同步Tool自有module/memory，未继续修改Go，未stage/commit。
