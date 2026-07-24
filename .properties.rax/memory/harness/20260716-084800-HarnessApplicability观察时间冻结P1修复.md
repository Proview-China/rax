# Harness Applicability观察时间冻结P1修复

时间：2026-07-16 08:48（Asia/Shanghai）

独立Review发现初版`CommittedPendingActionApplicabilityBindingV1`把`CheckedAtUnixNano`连同完整Reader Request封存，Runtime Adapter随后重放构造期时间；真实递增时钟下，Kernel的fresh读取会把正常延迟调用判成stale。

修复后Binding只canonical seal稳定`CommittedPendingActionSubjectV1`与Session/Turn source coordinates，`CheckedAt`、Reader返回的`CheckedUnixNano`和`ExpiresUnixNano`均不进入Binding identity。Adapter每次Inspect先采fresh clock生成本次Request，Reader以自己的fresh clock封存Checked/Expires；Reader返回后Adapter再采第二次fresh clock，只有`now >= checked && now < expires`且公共expiry不超过底层projection时才返回current。

新增反例覆盖不同CheckedAt保持同一Binding digest、构造后延迟的真实递增时钟、Reader后时钟回拨和TTL crossing。测试fixture仍只用于隔离验证，不代表生产composition root、Backend或SLA。
