# Cache Plan离线Inspect完成

时间：2026-07-17 15:20（Asia/Shanghai）

## 事件

Context Owner完成第六个Owner-local Offline SDK/API/CLI只读入口`InspectCachePlanV1`与`context cache inspect`。入口只接受调用者已构造的provider-neutral `CachePlan + ProviderCacheProfile + CheckedUnixNano`，复用现有合同核验Plan/Profile exact digest、Partition/Key、TTL/currentness与离线经济性。

返回的`Current=true`仅表示该离线输入闭包在checked time内有效；它不是Cache Entry current、Provider状态或cache hit。实现不生成Plan，不访问Provider，不创建CacheEntry/CacheAccessFact，不写Store，不创建Effect、Capability、Settlement或production root。

## 主要落点

- `sdk/cache_inspect.go`及tests：typed Request/Response、Seal/strict codec、canonical Request/Result/Inspection digest、48/48 MiB wire cap、cancel零Response与64并发确定性；
- `offlineapi/service.go`：第六typed方法与严格JSON dispatch；
- `cmd/context/main.go`：stdin/stdout只读`cache inspect`；
- `contract/cache.go`：Provider Profile expiry保持typed `expired`；Cache经济性仍使用防溢出精确算术；
- design/plan/module：同步六入口current truth及cache usage不等于hit边界。

## 实际验证

- `go test -count=100 ./contract ./sdk ./kernel ./offlineapi ./cmd/context`：PASS；
- `go test -race -count=20 ./contract ./sdk ./kernel ./offlineapi ./cmd/context`：PASS；
- `go test -count=1 ./...`：PASS；
- `go test -race -count=1 ./...`：PASS；
- `go vet ./...`：PASS；
- `gofmt`、Markdown相对链接、draw.io XML、import boundary、`git diff --check`：PASS。

增量反例覆盖Plan/Profile过期、exact ref/digest/Partition/Key漂移、递归duplicate key、null Plan、cancel、response tamper、usage/cache tokens冒充hit及64并发输出漂移。当前Go闭集hash：`89b4b06215b49a7dc8dc93fdc0e4dd6bd9e8ff203d327cbe47c447b41d585e6f`。

## 未解锁边界

Provider Cache真实create/read/write/warm/refresh/invalidate/delete、Cache Entry生产Backend、跨Owner Adapter、Capability和production composition仍为NO-GO；本事件不改变CTX-D07、Application/Harness/Model Invoker/Continuity公共合同缺口。
