# Agent Host H4 Assembly Publication 接线完成

时间：2026-07-18 11:05（Asia/Shanghai）

## 完成事实

- Harness Assembly Publication已经提供SQLite WAL durable Owner Store；staged四对象在commit marker前对historical/current reader不可见，publication marker、current history与scope current在同一事务CAS提交。
- Agent Host新增additive `CompiledAssemblyArtifactsV2`与`HarnessCompilerArtifactsV2`，直接保留同一次Harness compile产生的Generation、Manifest、Graph、Handoff public sealed对象；HostV1原合同不变。
- Agent Host新增production `AssemblyPublicationAdapterV2`。Generation、Manifest、Graph、Handoff和commit各自使用固定step/attempt写入HostJournalV2；只有本次intent CAS正常成功者允许调用Owner写口，lost reply/restart只Inspect exact staged/historical/current。
- 输出Runtime中立`OwnerCurrentRefV1`及完整Publication/Generation/Manifest/Graph/Handoff exact refs，供后续Binding与HostV2使用。

## 验证

- Adapter定向ordinary100与race20通过；
- Agent Host full ordinary、full race、vet、gofmt、import boundary和diff-check通过；
- 覆盖partial staged不可见、对象splice、stale predecessor/ABA、64个独立Adapter单写、TTL crossing、SQLite commit lost reply与双Store重启恢复。

## 未完成边界

- 尚未接入HostV2 production composition root；
- Runtime Binding、Activation、Generation Association、Application Start与SystemReady完整current闭包仍由后续波次完成；
- 当前结果不等于全6+1 production `SYSTEM_READY`。
