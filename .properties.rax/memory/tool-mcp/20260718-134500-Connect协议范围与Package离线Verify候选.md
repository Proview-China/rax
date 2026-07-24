# Connect协议范围与Package离线Verify候选

- official SDK Connect actual point新增协商协议范围门：必须位于exact MCP Server Descriptor的Minimum/Maximum范围且不高于模块稳定协议上限。
- Provider后发现版本越界时不产生Protocol Receipt或Connection，physical Entry进入inspect-only Unknown；已建立Session作为Residual保留，重投只Inspect且Provider调用总数保持1。
- driver记录的initialize bytes还必须逐字等于同一Session canonical InitializeResult；漂移同样进入Unknown。Connect路径删除自行`Session.Close()`，避免绕过独立Close Effect。
- 定向门：协议/response漂移ordinary×100与race×20通过；协议范围Fuzz 5秒约30万次输入通过。
- V1正式兼容窗口继续锁`2025-11-25`；`2025-06-18`只作为未来显式降级Conformance，不自动放宽。
- Tool Package形成离线Verify设计候选：OCI负责content-addressed Artifact，Sigstore Bundle负责离线验签材料，in-toto Statement绑定Artifact subject；Praxis保留Trust Policy current、Verification Fact/current与Registry状态。
- 该Package候选尚未设计终审，未写Go；Fetch/Install/Enable、production Artifact Store、在线透明日志和市场信任仍NO-GO。
- live可实现性审计发现Package Registry generic Transition未消费任何Verification current；候选已补Runtime-neutral Artifact/Trust Port、Tool Observation/Fact/current、Package Registry current及同事务`TransitionVerifiedPackageV1` P0。该P0闭合前Package admitted/active只视为owner-local fixture状态。
- 候选进一步拆开历史Trust Policy与immutable current lease，Observation/Fact的时间只由唯一Repository owner-clock在create-once线性化点写入，避免fresh clock进入stable ID导致并发冲突。
- 当前精确待裁决项是Package Registry current的15秒fail-closed候选上限与legacy generic Package admission的关闭方式；未终审前Package Verify Go门保持关闭。
- Owner-local Surface Compiler新增Registry Snapshot漂移反例：新Snapshot digest只能得到新Surface ID/Digest，旧Surface不原地修改；定向ordinary×100、race×20、surface full与vet通过。Application/Assembler Reconcile仍是跨Owner待接线项。
- Package revoke补齐SDK Assembly端到端反例：撤回后旧Snapshot因漂移失效，新Snapshot因Package revoked失效，且revoked不可重活；定向ordinary×100、race×20、SDK full与vet通过。
- MCP lifecycle状态机Fuzz落盘，5秒126,210次执行/共81个语料通过；覆盖错CAS、clock rollback、Snapshot digest漂移、Closed与Snapshot终态不复活、失败零写和revision单调。
- 上述Surface/Package/Lifecycle反例加入后，Tool/MCP模块full ordinary、full race、vet、`go mod tidy -diff`、production import boundary与`git diff --check`全部通过。首次import搜索命令误命中Conformance测试中的禁止字符串而退出1；改为只扫描非测试生产Go文件后PASS，不是代码失败。
- 阶段9补齐legacy Port、Surface、Package与MCP协议的迁移/回退矩阵：禁止旧Port包装升权，漂移使用新exact lineage/revision与新Run，不原地换面、不删除外部Effect历史。
