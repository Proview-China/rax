# Runtime Surface Registry Exact Reader第六审返修

- 时间：2026-07-16 22:06:19 +08:00
- 状态：Surface第六审发现Registry exact Reader缺失P0，Runtime asset-only返修已落盘，等待复审；Go nominal/Reader均未实现。

本次资产新增唯一`RegistrySnapshotExactReaderV1`候选。请求只能是完整`RegistrySnapshotRefV1{Owner,ContractVersion,ID,Revision,Digest}`；Reader由`Owner`指向的Registry Authority Owner实现，先读取immutable historical record，再验证Owner current pointer exact，最后返回deep clone。旧historical Ref存在但已非current时返回PreconditionFailed，不得用于pre-dispatch。

错误闭表为InvalidArgument、NotFound、Conflict、PreconditionFailed、Unavailable、Indeterminate；Unavailable/Indeterminate不得降级。Reader不暴露Registry内容、Publish/CAS/mutation，也不授Provider/Prepared/Permit/Enforcement权力。当前只有资产，无Runtime Go nominal、Reader、Store、production root/backend/SLA，不stage、不commit。
