# ContextOfflineSDKV1第二轮审计返修候选

时间：2026-07-17 00:30（Asia/Shanghai）

第二轮独立代码审计结论为`NO，P0=2/P1=4/P2=0`。本事件只记录Context Owner范围内的第二轮返修候选，不把owner自验写成独立复审YES，不改变Application、Harness、Runtime、Tool或production C层状态。

本轮闭合：typed四入口在任何request/bundle clone前完成required/count/token/raw预检，成功预检路径零分配；wire先按hard cap流式定位meta，再在payload clone和DTO Decode前执行request-specific wire cap及递归token扫描，base64 chunk count、单item/aggregate raw和non-content wire超限均在`[]string`物化前fail closed；canonical JSON、Digest及非正文Response Encode使用context-aware writer并至少每64 KiB检查cancel/deadline；Rules/Decisions/Fragments及bundle items的null被拒绝，admitted Decision必须显式携Region、非admitted禁止Region；Preview/Inspect typed预检使用Compile-derived 76 MiB而非100 MiB global guard。

新增反例覆盖typed preflight零分配成功路径和clone前超限、超大base64 chunk vector pre-materialization拒绝、request-specific wire cap、wire/canonical mid-cancel、Rules/Decisions/Fragments null，以及Decision Region条件presence。既有base64 0/1/2/48 KiB -1/exact/+1/96 KiB、renderer 48/64 KiB及4 MiB、workspace fault、Compile 24/52/76与global 68/100矩阵继续保留。

最终候选实跑：`go test ./... -count=100` PASS；`go test -race ./... -count=20` PASS；`go vet ./...` PASS。max-size fixture input=`25,165,824`、generated=`53,129,367`、output=`78,295,191`、wire=`104,407,083` bytes，1/2/4/8并发PASS，最高VmHWM=`3,285,408 KiB`，mid-cancel=`807.57 µs`。按仓库根标准命令计算的当前Context Go闭集hash=`9a30ea418bc44b9d1a10a777fa5926f3c7a083c35d082036f30a85ebdd0c1d61`。

当前状态保持`repair_candidate / independent_reaudit_pending`；下一轮独立复审YES前不得标记implementation YES，不得解锁G6B、Capability、Harness Continuation、Turn推进或production root。
