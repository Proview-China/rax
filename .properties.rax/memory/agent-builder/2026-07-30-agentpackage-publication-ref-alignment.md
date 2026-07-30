# AgentPackage 与 Harness Publication 精确坐标对齐

## 结论

AgentPackage 的 Generation、Manifest、Graph、Handoff 锁定引用必须直接等于 Harness `AssemblyPublicationV2` 的规范 artifact refs。

旧实现用 `generationID/manifest|graph|handoff` 自行生成后三个对象 ID；Harness Publication Owner 使用由 `AssemblyInputDigest + GenerationID` 推导的 `publicationID/manifest|graph|handoff`。两套坐标各自可验证，但不能让 Package Loader 读取同一份 Owner 历史资产。

## 落地

- AgentPackage Compiler 复用 `DeriveAssemblyPublicationIDV2`；
- Lock 校验要求 Manifest、Graph、Handoff 使用 PublicationID 且 revision 为 create-once revision `1`；
- 黑盒直接构建真实 `AssemblyPublicationBundleV2`，断言 Package Lock 的四个 exact refs 与 Publication artifact refs 完全相等；
- 不改变 Definition、Resolved Plan、Binding、Runtime、Host 或 Console 合同。

## 后续边界

Harness Publication Owner 仍需提供窄的 exact artifact reader adapter，AgentPackage Loader 才能从持久历史发布中加载闭包。该 reader 不归 Agent Builder 所有，也不得复制 Harness artifact Store。
