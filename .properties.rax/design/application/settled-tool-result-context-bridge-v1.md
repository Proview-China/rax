# Settled ToolResult 到 Context production bridge V1

状态：exact-chain owner-local adapter 已实现；production Context bridge 与 request builder 继续 BLOCKED/NO-GO。

## Owner 与已完成边界

- Tool Owner：DomainResult、ApplySettlement、ToolResult、SettledToolResultProjection current。
- Runtime Owner：Inspection、Settlement、Association、UnknownOutcome。
- Context Owner：ContentRef、Sensitivity、TokenEstimate、Recipe、CacheIdentity、Generation Current。
- Application Owner：SingleCall result、TurnContinuation Attempt 与 coordination。
- Harness Owner：ActiveContext CAS。

新增 applicationadapter/settled_context_exact_v1.go 只验证同一 current 窗口内的 Application Result、ToolResult、DomainResult、Projection 与 Runtime V4 closure。它校验 exact ID/revision/digest、Apply、Inspection、Association、Schema、Payload、Attempt、AuthoritativeTime，并将有效期截断为 max checked 到 min expires。它不是公共 Wire DTO，不创建任何 Context-owned 字段。

## 两个生产硬缺口

### D1 Tool Owner projection producer/current reader

生产树只有 SettledToolResultProjectionV1 合同，没有 durable producer/store/current reader。Tool Owner仍需冻结：inline/artifact来源、Projection.Tool的Candidate/Descriptor provenance、Classification来源、TTL截断、lost reply原Result Inspect和重启conformance。Projection.Tool无法从当前Application request/result唯一证明；本切面不脑补。

### D2 Context Owner refresh input current/builder

ContextTurnRefreshRequestV1还需要ParentSource、ExpectedCurrent、Recipe、CacheIdentity、ContentRef物化、Sensitivity、TokenEstimate与stable prefix TTL。现有生产树没有权威Reader同时提供这些值；OperationInspectionSettlementRefV4自身也没有可无歧义映射到legacy FactRef的独立ID。禁止从Classification猜Sensitivity、从payload长度猜生产TokenEstimate、从Runtime Projection ID拼Inspection FactRef。

## 稳定 Attempt 顺序

D1/D2完成后必须：Inspect Harness source/ActiveContext；Inspect exact Tool chain；Inspect Context input current并密封完整Context body；调用DeriveTurnContinuationAttemptIDV1；把同一Attempt写入Context coordination；Seal Context prepare并取得exact ExpectedContextRefreshAttempt；最后Seal TurnContinuationStart。任一Tool、ActiveContext、Context body、TTL或target Turn splice都必须在Begin前失败。

## Lost reply、TTL与黑盒

Tool projection create、Context Prepare/Apply、Harness Begin/Commit的lost reply只Inspect原command/attempt，禁止换key重放。NotAfter取Tool projection、Runtime Inspection、Application result、Harness source/ActiveContext、Context input current的最早expiry；任何阶段TTL crossing都fail closed且下一Model Turn保持关闭。

最终黑盒必须覆盖：真实exact happy path；ToolResult/DomainResult/Apply/Inspection/Association splice；Projection Tool/artifact/classification splice；Parent/Recipe/Cache/current漂移；同Attempt异payload；各阶段lost reply；逐阶段TTL crossing；重启后不重复Tool effect或Context generation。
