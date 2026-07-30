# Workspace Read Exact Current V1 Module

入口为 `WorkspaceReadCurrentProjectionReaderV1.InspectWorkspaceReadCurrentV1`。

输入是带 sealed Runtime Physical Authorization 的 exact query；输出是 Sandbox-owned 只读 current projection。模块只读取四个 Owner current，不写数据库、不调用 Provider、不签发 Runtime 或 Review 事实。

`SemanticDigest`用于判断 exact closure 是否保持一致；`ProjectionDigest`用于验证一次完整响应没有被修改。
