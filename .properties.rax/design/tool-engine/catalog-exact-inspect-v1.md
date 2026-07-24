# Registry Catalog Exact Inspect V1

## 1. 目标

在既有transport-neutral Catalog分页外，提供exact Registry Object只读投影，使开发者能够按
`kind + ID + revision + digest`检查Capability、Tool、Package或Tool Alias，而不直接依赖
Registry实现、不读取latest弱坐标，也不获得Admission、Transition、Provider或网络能力。

## 2. 公共对象

`InspectRegistryObjectRequestV1`：

- `Kind`闭集：`capability|tool|package|tool-alias`；
- `Exact`：Tool Owner `ObjectRef{ID,Revision,Digest}`；Alias只在Reader调用处无损转成
  `ToolAliasRefV1`，不复制另一套事实。

`RegistryObjectProjectionV1`是closed typed union：

- `ContractVersion`、`Kind`、完整`registry.Record`；
- `Capability|Tool|Package|ToolAlias`恰好一个非nil；
- `ProjectionDigest`覆盖上述完整canonical body并排除自身。

Record的Kind/ID/ObjectRevision/ObjectDigest必须逐字段绑定typed object。不能用`any`、裸JSON、
Kind翻译或同ID其他类型补位。

## 3. 读取与错误语义

1. API先验证non-nil/non-canceled context和exact request；
2. 经同一注入的SDK exact Reader执行S1；
3. 再读同一kind/exact ref执行S2；
4. 两次ProjectionDigest不一致返回Conflict/BindingDrift；
5. 返回值深拷贝slice，调用方修改不影响Owner Store；
6. NotFound、Unavailable、Canceled与typed Owner错误原样保留，不把NotFound解释成未执行、未注册或可创建。

该投影是Observation/read model，不授Authority、Review、Admission、current execution资格，
也不替代Tool Surface、Binding或Runtime actual-point Reader。

## 4. 包与依赖

- 实现：`ExecutionRuntime/tool-mcp/api/catalog_inspect_v1.go`；
- 单元/漂移/并发：`api/catalog_inspect_v1_test.go`；
- 黑盒JSON往返：`tests/blackbox/catalog_inspect_v1_test.go`；
- Conformance/import：`tests/conformance/sdk_v1_test.go`；
- 允许依赖：Tool `contract/registry/sdk`与Runtime public `core/ports`；
- 禁止依赖：Application/Harness/Model/Context实现、Runtime kernel/fakes/internal、official
  MCP SDK、网络、进程、数据库、Provider与production root。

## 5. 反例

- wrong kind、wrong digest、cross-kind type-pun；
- union为零项或多项、Record/object坐标漂移、ProjectionDigest篡改；
- S1/S2间Registry state/revision漂移；
- typed-nil、nil/canceled context；
- 返回slice修改污染Store；
- 64并发exact读产生不同ProjectionDigest；
- Catalog新增Register/Transition/Call/Connect/Listen/Serve等写入或Transport方法。

