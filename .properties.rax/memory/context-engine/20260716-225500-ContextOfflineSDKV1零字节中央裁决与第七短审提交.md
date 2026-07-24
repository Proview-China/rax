# Context Offline SDK V1零字节中央裁决与第七短审提交

时间：2026-07-16 22:55（Asia/Shanghai）

中央裁决唯一Delta：保持live `ContentRef.Validate`的`Length>0`语义，不修改全局ContentRef，也不建立第二套nominal。

零字节只属于独立base64 primitive golden：`encode([]byte{})=[]`、`decode([]string{})=[]byte{}`。该primitive结果没有ContentRef，不能构造`OfflineContentItemV1`，也不能进入`OfflineContentBundleV1`或private wire item。Bundle中每个Ref必须通过live Validate且`Length>0`，bytes必须非空并与Ref Length/Digest exact。

硬反例：零长度Ref、空bytes或`base64_chunks=[]` wire item构造必须返回`invalid_argument`和零Bundle；required空内容没有合法非零ContentRef时Fail Closed，不能借空item或primitive空编码绕过。Optional合法Ref缺失继续复用live `content_unavailable` Residual；required合法Ref missing只在SDK边界映射`not_found`。

第六审YES基线保持；本澄清已同步到Context design/plan/test matrix并提交第七独立短审，当前`review_pending`。第七审YES及中央明确授权前不写Go；production C层继续NO-GO。

本事件为append-only新增；未修改旧memory、Go或其他Owner资产。
