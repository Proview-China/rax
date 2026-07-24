# Sandbox Workspace真实Overlay捕获

时间：2026-07-18 12:29:50 +08:00

## 事件

- 新增Sandbox Owner本地`workspacefs` Driver与窄`WorkspaceChangeSetCapturePortV1`。
- trusted config把WorkspaceView exact ref绑定到Base/Overlay/Blob roots；路径不进入公共DTO。
- Driver执行Base/Overlay S1读取、scope/hidden/symlink/special-file/limit检查、content-addressed blob
  create-once、Base/Overlay S2复读，再生成canonical staged ChangeSet。
- read-only、hidden、symlink、base revision与capture期间host drift全部fail closed；任何失败返回零
  ChangeSet。Driver无commit、Provider、Review、Evidence、Settlement或Runtime写权限。

## 验证

- 真实文件modify/add与blob落盘、只读/隐藏/symlink/base drift、S1/S2 host drift反例PASS。
- 64并发同内容capture返回同一ChangeSet digest并只形成一个blob；ordinary count100和race count20 PASS。

## 边界

- `praxis.sandbox/workspace-commit`仍必须经过独立Runtime Operation/双Enforcement/Evidence/Inspect/
  DomainResult/Settlement/Apply；当前Runtime Evidence闭表没有该run-scoped profile，不能用普通文件写或
  Host executor回包冒充commit。
