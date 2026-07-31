# Governed V3 Provider Injection Compare A2 实现候选

## 状态

- 基线：`db0647b94bdb0186b60bbc6ca684b63224415ec2`；
- 分支：`agent/governed-v3-provider-injection-compare-a2`；
- Owner：Model Invoker；
- 当前产物：未提交实现候选，等待独立审计。

## 已闭合

- `prepareAt`成功并安装lease retire defer后计算A1 actual digest；
- 与S2 exact Prepared事实逐字节比较；
- fixture不再使用占位hash，而是根据concrete Provider/Protocol request计算；
- Provider、Protocol、Tools、ToolChoice、Parallel与ProviderOptions漂移fail closed；
- Parameters key order/whitespace与paired-surrogate/literal-scalar等价输入不误报；
- release错误保留证据；
- Boundary、guard、Provider与Observation保持0。

## 保留边界

- exact match仍命中Credential current HARD NO-GO；
- V3 Material禁止ProviderOptions进入RouteCall；其正向canonical等价由A1套件证明，A2不伪造不可达路径；
- 不创建Boundary，不调用Provider；
- Credential current、durable Provider Attempt/Observation和production composition未实现；
- 本事件不能作为真实Provider链已解锁的证据。

## 验证

- targeted ordinary×100：通过；
- targeted race×20：通过；
- Model Invoker full ordinary：通过；
- Model Invoker full race：通过；
- `go vet ./...`：通过；
- routegateway与A1 import boundary：通过；
- `gofmt`与`git diff --check`：通过。

上述结果只证明A2 owner-local reference切片达到独立审计候选门禁，不代表production eligible，也不解除Credential、Provider Attempt/Observation与真实Provider链的HARD NO-GO。
