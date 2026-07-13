# 执行语义并集第二轮 Review 与测试加固计划 v2

## 1. 状态与授权

- 模块：`model-invoker`
- 创建时间：2026-07-13
- 当前状态：陈旧计划（已完成）
- 授权依据：用户要求在提供真实 API 与订阅账号前，再做一次代码 review/走查，细化单元测试并加强集成测试，避免把真实调用额度消耗在可离线发现的问题上。
- 凭据边界：不读取、不复用、不打印、不调用真实 API Key、OAuth、订阅登录态或官方二进制；真实联调继续保持`not_run`。
- 复用事实源：[执行语义并集 Runtime 实施计划 v1](./execution-semantic-union-runtime-v1.md)及其 design/module/memory 资产。

本计划不新增模块、不改变已确认的五顶层原语和 Intent/Mechanism/Effect 方向。它只对已实现 v1 做第二轮独立审查、补反例、修复被测试证明的合同缺口，并把真实联调入口推进到“凭据到达即可逐路执行”。

## 2. 本轮可预见产物

1. `union/profile/effect` 的边界值、无效状态、合成策略和观察器反例；
2. `execution/direct` 的密封权限、交叉身份、状态迁移、并发、关闭和错误顺序测试；
3. 五路 Harness 的 framing、EOF、快速响应、取消、权限、Manifest、进程树和隔离故障矩阵；
4. 使用真实 Adapter 实现但只连接 fake process/backend 的跨 Route 本地集成链；
5. 更新后的统一离线门禁、覆盖率、fuzz、benchmark 和 review 结论；
6. module、plan、memory 与真实联调清单同步。

## 3. Review 原则

- 先证明缺口，再修改代码；不以覆盖率数字驱动无意义测试。
- 单元测试覆盖一个不变量或一个失败分类，错误必须断言到语义而非只断言非空。
- 集成测试必须跨越至少两个真实包边界；只调用内存 fake 接口不冒充 Adapter 集成。
- fake process/backend 必须显式注入、仅使用 loopback/临时目录、空登录 HOME 和清空后的凭据环境。
- Adapter、Provider、Harness 仍无权生成 Effect、Verification 或统一终态。
- `implemented_offline`、`offline_contract_tests=passed`与`live_verification=not_run`继续分轴记录。

## 4. 计划清单

### A. 现场与矩阵

- [x] 复核工作区、既有测试数量、覆盖率低分支和上一轮残余风险；
- [x] 建立语义、Runtime、Harness 三条独立审查线；
- [x] 形成缺口清单并按 P0/P1/P2 分级。

### B. 语义/Profile/Effect 单元测试

- [x] 补 Request/Plan/Event/Command/Result 的 tagged union、clone、digest 和非法组合反例；
- [x] 补 Profile 多层收紧、空交集、排序稳定性、Manifest opaque/absent 和 fallback 性质测试；
- [x] 补文件类型矩阵、symlink/root、move、metadata/diff/hash 边界；
- [x] 补 JSON Schema/repair、Tool/Process/Computer evidence 与 satisfaction 反例；
- [x] 运行相关 normal/shuffle/race/fuzz/vet。

### C. Runtime/Direct 单元与并发测试

- [x] 复核 Plan-bound Ledger 的 Intent/Plan/Attempt 交叉身份与 Adapter authority；
- [x] 补状态迁移、审批 TTL/revision/idempotency、取消/静止/协调、唯一终态和 replay 反例；
- [x] 补 Direct 非流/流、Tool continuation、未知工具、断流、Close 与错误优先级；
- [x] 加入恶意 Adapter、并发 Command/Receive/Close 和确定性重放测试；
- [x] 运行相关 normal/shuffle/race/vet。

### D. Harness 黑盒与本地集成

- [x] 补 JSONL/JSON-RPC/ACP 半帧、超限、重复 ID、晚响应、EOF 和取消竞态；
- [x] 补 process env/cwd/digest、stdout/stderr、进程组与幂等 Close 故障矩阵；
- [x] 补 Codex/Claude/Gemini/Kimi/Qwen 权限、Manifest、工具、终态和多会话隔离；
- [x] 使用五路真实 Adapter + fake process 完成相同请求的 Preflight→Open/Runtime→Reconcile→Verify→Result 集成；
- [x] 验证 integration build tag、三重 live gate、凭据拒绝和默认安全 Skip。

### E. 最终验收与同步

- [x] `gofmt`、`go mod tidy -diff`、`go mod verify`、`go vet`；
- [x] 全仓 normal、shuffle、race 和统一离线脚本；
- [x] 记录最终 coverage、定向 fuzz 和 benchmark；
- [x] 独立复审新增测试是否真正命中生产分支；
- [x] 更新 module/plan/memory/index；
- [x] 真实 API、OAuth、订阅和官方二进制保持`not_run`。

## 5. 完成条件

只有新增审查项都形成“测试证明的修复”或“有证据的无缺口结论”，全部离线门禁在最终代码上重跑通过，且真实联调输入清单可逐 Route 直接使用，本计划才转为“陈旧计划（已完成）”。

## 6. 完成记录

第二轮独立审查已完成，并修复了测试实际证明的 P0/P1 合同缺口：

- `union/profile/effect`：封闭 v1 tagged union、非法 extension 与高置信凭据拒绝；文件根、Move 目标、symlink、Effect/Verification 双向关系、supersession、Computer readback、typed-nil repair 和无效 UTF-8摘要均有反例；
- `execution/direct`：Plan-bound Ledger 拒绝跨 Intent/Plan/Attempt 与未知因果引用；Runtime 拒绝 Session/Turn 身份伪造并补齐 Effect/Verification 关联；Direct 的最终响应工具调用、并发重复结果、名称篡改、非法副作用状态、Close 与终态顺序均已锁定；
- Harness：Claude/Qwen 结构化输出补齐 Attempt；Attempt ID 按 Execution 隔离；五个生产 Adapter 通过 fake child process 走通完整 Runtime，并验证同一 Adapter 的并发 Execution/Session/Profile 隔离；
- live smoke 的默认 Skip、integration tag、全局/单 Route 开关、空 HOME、代理清除和高置信明文凭据门禁均进入统一离线入口。

最终验证结果：普通、五轮全仓 shuffle、全仓 race、integration-tag 五路集成 race、统一离线脚本、vet、格式与 module 校验均通过；默认全仓语句覆盖率为 `76.6%`，合并 integration profile 后为 `76.7%`。五项定向 fuzz 共执行 `1,171,314` 次并通过。三轮基准范围为：Profile compile `1.340-1.541 ms/op`，Manifest diff `102.9-247.3 us/op`，256-event replay `3.068-3.337 ms/op`，文件快照 `118.2-161.0 us/op` / `534.33-727.69 MB/s`。

本轮不把以下 P2 边界伪装成已解决：高熵用户正文不是凭据扫描对象；文件 Observer 的 authorize-then-use TOCTOU 需要未来 Sandbox/fd/openat 执行层闭合；新增厂商 ContentPart 必须升级版本化合同。真实 API、OAuth、订阅账号和官方二进制仍为 `not_run`。

后续增量：2026-07-13已执行Codex Pro单Route临时登录实测，官方CLI与Praxis App Server均成功；其余Route继续为`not_run`。该增量不重写本陈旧计划的历史完成状态，详见module与memory记录。
