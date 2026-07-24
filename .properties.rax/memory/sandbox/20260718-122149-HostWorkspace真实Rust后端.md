# Sandbox Host Workspace真实Rust后端

时间：2026-07-18 12:21:49 +08:00

## 事件

- 用户持续Goal要求完成`tmp.document/Sandbox.md`全范围；本切片闭合Host Workspace真实执行面。
- Rust Data Plane新增bwrap Host Provider与root配置：workspace/tool仅接受opaque binding，caller不传
  宿主路径；tmpfs root只读挂载Owner允许的toolchain与精确workspace overlay，默认独立user/PID/
  IPC/UTS/network namespace并clearenv。
- Provider实现allocate、activate/open、Inspect、Fence、Release、Cleanup；PID与`/proc`start-time共同
  形成process identity，live进程拒绝Release，PID复用/identity丢失不推导安全终态。
- Go wire新增strict Host payload与response provider identity/最短TTL校验。Provider仍只返回
  Observation/Receipt，不创建Workspace Fact/ChangeSet、不执行commit、不写Runtime/Sandbox权威事实。

## 验证

- 本机真实bwrap执行、workspace写入、宿主home不可见、独立Inspect/Fence/Release/Cleanup通过。
- symlink workspace、tool digest漂移、非法路径/网络/TTL/NUL payload均fail closed。
- Host fence并发反例定向100轮PASS；Rust debug/release全目标测试、strict clippy/fmt PASS。
- Sandbox Go full ordinary/race与vet PASS。

## 仍未完成

- production Host binding/readiness/Secret/network allow-list与部署认证。
- MicroVM、Remote、Checkpoint/Restore、真实Workspace governed commit、SDK/CLI/API、Assembly与系统SLA。
- Checkpoint live public Operation/Evidence/Settlement合同存在，但ordinary V4 Sandbox current projection
  不能表达checkpoint phase graph；已在Sandbox Port Delta记录additive actual-point sibling需求，缺口关闭前
  Checkpoint Provider调用保持0。
