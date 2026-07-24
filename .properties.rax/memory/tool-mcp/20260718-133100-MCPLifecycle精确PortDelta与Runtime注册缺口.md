# MCP Lifecycle精确Port Delta与Runtime注册缺口

- 时间：2026-07-18 13:31 +08:00
- 状态：联合候选；Tool Go门关闭；production NO-GO。

Tool Owner把ordinary Call Cancel、Drain、Connection Close收敛为精确Port Delta：Cancel使用
Run/Session/Turn/Action/Context五维独立Effect，Close使用Run/Session独立Effect，Drain只做Tool
Owner本地CAS。Delta列出active-call current、Route、Authorization Request/Authorization、
physical Port、Drain Store、恢复与反例；未获得Runtime/Application联合终审前不实现写入口。

live只读审计发现Runtime已发布Discovery Page矩阵类型，但通用Matrix Validate与Policy subject
注册闭集没有登记`praxis.mcp/discover`。该缺口不否定现有owner-local专属测试，但阻断production
统一治理；Tool不越权修改Runtime，也不私建兼容注册表。

