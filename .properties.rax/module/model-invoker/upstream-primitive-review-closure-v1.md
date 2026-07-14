# 全上游统一原语层Review闭环 v1

## 作用

本切片不是新增第二套模型请求，而是给既有LLM、Harness、外围、本地和Relay实现增加统一的“可达性证明与结果合同”。入口为`semanticmatrix.BuildUnion`。

## 组成

- `semanticmatrix/union.go`：780条LLM能力行之外的206条Surface行；
- `operation/specs/registry.go`：14个外围HTTP Canonical Surface；
- `realtime/specs/registry.go`：3个官方Realtime Canonical Surface；
- `provider/localcompat/registry.go`：3类本地/企业自建产品；
- `realtime/{registry,invoker,validate}.go`：统一Realtime选择、校验、事件和关闭；
- `operation/invoker.go`：统一外围流生命周期、结果身份与Artifact校验；
- `provider/localcompat/adapter.go`：自建模型同步/流式响应Model证明。

## 关键合同

1. Provider SDK和HTTP/WebSocket方言不越过公共原语类型；
2. LLM最终Route身份由RouteGateway拥有；
3. Harness差异由Profile编译到Intent/Mechanism/Effect；
4. Operation Stream固定由Invoker产生一个started和一个completed/error终态；
5. Realtime由Registry/Invoker选择Provider，Session事件重新编号并防御性复制；
6. Operation/Realtime请求和Operation结果不与调用方或Provider共享可变内存；
7. Local与Relay不冒充官方Provider；
8. Canonical Surface缺失或漂移直接导致UnionMatrix校验失败。

## 离线验收

- 联合矩阵：780条LLM行、206条Surface行；
- 全仓普通测试、Shuffle、Race、Vet通过；
- integration-tag全量通过，真实外部调用按环境开关跳过；
- 未消费用户API或订阅额度。
