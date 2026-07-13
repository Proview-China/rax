package union

import (
	"encoding/json"
	"time"
)

type CapabilityOrigin string

const (
	CapabilityOriginNative         CapabilityOrigin = "native"
	CapabilityOriginProviderHosted CapabilityOrigin = "provider_hosted"
	CapabilityOriginHarnessHosted  CapabilityOrigin = "harness_hosted"
	CapabilityOriginCallerHosted   CapabilityOrigin = "caller_hosted"
	CapabilityOriginEmulated       CapabilityOrigin = "emulated"
	CapabilityOriginUnavailable    CapabilityOrigin = "unavailable"
)

type SemanticFidelity string

const (
	SemanticFidelityExact       SemanticFidelity = "exact"
	SemanticFidelityTransformed SemanticFidelity = "transformed"
	SemanticFidelityDegraded    SemanticFidelity = "degraded"
	SemanticFidelityUnavailable SemanticFidelity = "unavailable"
)

type ExecutionOwner string

const (
	ExecutionOwnerModel    ExecutionOwner = "model"
	ExecutionOwnerProvider ExecutionOwner = "provider"
	ExecutionOwnerHarness  ExecutionOwner = "harness"
	ExecutionOwnerPraxis   ExecutionOwner = "praxis"
	ExecutionOwnerExternal ExecutionOwner = "external"
)

type ExecutionKind string

const (
	ExecutionKindAuto  ExecutionKind = "auto"
	ExecutionKindModel ExecutionKind = "model"
	ExecutionKindAgent ExecutionKind = "agent"
)

type EventOrigin string

const (
	EventOriginPraxis   EventOrigin = "praxis"
	EventOriginModel    EventOrigin = "model"
	EventOriginProvider EventOrigin = "provider"
	EventOriginHarness  EventOrigin = "harness"
	EventOriginExternal EventOrigin = "external"
)

type EventFamily string

const (
	EventFamilyLifecycle  EventFamily = "lifecycle"
	EventFamilyIntent     EventFamily = "intent"
	EventFamilyMechanism  EventFamily = "mechanism"
	EventFamilyModel      EventFamily = "model"
	EventFamilyItem       EventFamily = "item"
	EventFamilyEffect     EventFamily = "effect"
	EventFamilyControl    EventFamily = "control"
	EventFamilyDiagnostic EventFamily = "diagnostic"
)

type Visibility string

const (
	VisibilityModelVisible   Visibility = "model_visible"
	VisibilityUserVisible    Visibility = "user_visible"
	VisibilityProgressOnly   Visibility = "progress_only"
	VisibilityAuditOnly      Visibility = "audit_only"
	VisibilityPrivateRuntime Visibility = "private_runtime"
)

type SecurityClassification string

const (
	SecurityPublic     SecurityClassification = "public"
	SecurityInternal   SecurityClassification = "internal"
	SecuritySensitive  SecurityClassification = "sensitive"
	SecurityRestricted SecurityClassification = "restricted"
)

type VerificationStatus string

const (
	VerificationPending           VerificationStatus = "pending"
	VerificationVerified          VerificationStatus = "verified"
	VerificationPartiallyVerified VerificationStatus = "partially_verified"
	VerificationUnverified        VerificationStatus = "unverified"
	VerificationContradicted      VerificationStatus = "contradicted"
	VerificationNotApplicable     VerificationStatus = "not_applicable"
)

type ExecutionStatus string

const (
	ExecutionStatusSucceeded     ExecutionStatus = "succeeded"
	ExecutionStatusPartial       ExecutionStatus = "partial"
	ExecutionStatusFailed        ExecutionStatus = "failed"
	ExecutionStatusCancelled     ExecutionStatus = "cancelled"
	ExecutionStatusIndeterminate ExecutionStatus = "indeterminate"
)

type AttemptStatus string

const (
	AttemptStatusPlanned          AttemptStatus = "planned"
	AttemptStatusSelected         AttemptStatus = "selected"
	AttemptStatusAwaitingApproval AttemptStatus = "awaiting_approval"
	AttemptStatusRunning          AttemptStatus = "running"
	AttemptStatusCompleted        AttemptStatus = "completed"
	AttemptStatusFailed           AttemptStatus = "failed"
	AttemptStatusDeclined         AttemptStatus = "declined"
	AttemptStatusCancelled        AttemptStatus = "cancelled"
	AttemptStatusIndeterminate    AttemptStatus = "indeterminate"
)

type ItemStatus string

const (
	ItemStatusPending       ItemStatus = "pending"
	ItemStatusInProgress    ItemStatus = "in_progress"
	ItemStatusCompleted     ItemStatus = "completed"
	ItemStatusIncomplete    ItemStatus = "incomplete"
	ItemStatusFailed        ItemStatus = "failed"
	ItemStatusCancelled     ItemStatus = "cancelled"
	ItemStatusIndeterminate ItemStatus = "indeterminate"
)

type SideEffectState string

const (
	SideEffectNone       SideEffectState = "none"
	SideEffectPossible   SideEffectState = "possible"
	SideEffectObserved   SideEffectState = "observed"
	SideEffectReconciled SideEffectState = "reconciled"
	SideEffectUnknown    SideEffectState = "unknown"
)

type IntentSatisfactionStatus string

const (
	IntentSatisfied          IntentSatisfactionStatus = "satisfied"
	IntentPartiallySatisfied IntentSatisfactionStatus = "partially_satisfied"
	IntentUnsatisfied        IntentSatisfactionStatus = "unsatisfied"
	IntentContradicted       IntentSatisfactionStatus = "contradicted"
)

type SelectionAuthority string

const (
	SelectionAuthorityRuntime        SelectionAuthority = "runtime"
	SelectionAuthorityModelWithinSet SelectionAuthority = "model_within_set"
	SelectionAuthorityHarness        SelectionAuthority = "harness"
	SelectionAuthorityProvider       SelectionAuthority = "provider"
)

type CommandKind string

const (
	CommandApproveAction     CommandKind = "approve_action"
	CommandDenyAction        CommandKind = "deny_action"
	CommandProvideInput      CommandKind = "provide_input"
	CommandCancelExecution   CommandKind = "cancel_execution"
	CommandInterrupt         CommandKind = "interrupt_execution"
	CommandContinue          CommandKind = "continue_execution"
	CommandProvideToolResult CommandKind = "provide_tool_result"
)

type ProfileSelector struct {
	Exact       *VersionedIdentity `json:"exact,omitempty"`
	Constraints map[string]string  `json:"constraints,omitempty"`
}

type ContentPart struct {
	Kind      string            `json:"kind"`
	Text      string            `json:"text,omitempty"`
	JSON      json.RawMessage   `json:"json,omitempty"`
	Reference string            `json:"reference,omitempty"`
	MediaType string            `json:"media_type,omitempty"`
	Name      string            `json:"name,omitempty"`
	SchemaID  string            `json:"schema_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type InputItem struct {
	ID       ItemID          `json:"id"`
	Kind     string          `json:"kind"`
	Role     string          `json:"role,omitempty"`
	ActionID ActionID        `json:"action_id,omitempty"`
	Content  []ContentPart   `json:"content,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

type Instruction struct {
	ID             string        `json:"id"`
	Authority      string        `json:"authority"`
	Scope          string        `json:"scope"`
	Content        []ContentPart `json:"content"`
	ConflictPolicy string        `json:"conflict_policy"`
}

type ContextReference struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Reference  string `json:"reference"`
	Snapshot   string `json:"snapshot,omitempty"`
	Access     string `json:"access"`
	Visibility string `json:"visibility"`
	Required   bool   `json:"required"`
}

type ToolDefinition struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	Kind           string          `json:"kind"`
	InputSchema    json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema   json.RawMessage `json:"output_schema,omitempty"`
	ExecutionOwner ExecutionOwner  `json:"execution_owner"`
	SideEffects    []string        `json:"side_effects,omitempty"`
	ApprovalPolicy string          `json:"approval_policy,omitempty"`
	Timeout        time.Duration   `json:"timeout,omitempty"`
	Extension      json.RawMessage `json:"extension,omitempty"`
}

type ToolPolicy struct {
	AllowedToolIDs  []string `json:"allowed_tool_ids,omitempty"`
	DefaultApproval string   `json:"default_approval"`
	Parallelism     int      `json:"parallelism,omitempty"`
	MaxActions      int      `json:"max_actions,omitempty"`
	NetworkPolicy   string   `json:"network_policy,omitempty"`
	WorkspacePolicy string   `json:"workspace_policy,omitempty"`
}

type OutputContract struct {
	AcceptedContentKinds []string        `json:"accepted_content_kinds,omitempty"`
	TextRequired         bool            `json:"text_required,omitempty"`
	JSONSchema           json.RawMessage `json:"json_schema,omitempty"`
	ArtifactKinds        []string        `json:"artifact_kinds,omitempty"`
	PatchFormat          string          `json:"patch_format,omitempty"`
	CompletionMode       string          `json:"completion_mode,omitempty"`
}

type ReasoningIntent struct {
	Effort       string `json:"effort,omitempty"`
	BudgetTokens int64  `json:"budget_tokens,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Observable   string `json:"observable,omitempty"`
}

type SessionIntent struct {
	Mode            string            `json:"mode"`
	SessionID       SessionID         `json:"session_id,omitempty"`
	TurnID          TurnID            `json:"turn_id,omitempty"`
	ExpectedProfile VersionedIdentity `json:"expected_profile,omitempty"`
	ExpectedRoute   VersionedIdentity `json:"expected_route,omitempty"`
}

type ExecutionPolicy struct {
	Stream               bool     `json:"stream,omitempty"`
	Sandbox              string   `json:"sandbox,omitempty"`
	CWDReference         string   `json:"cwd_reference,omitempty"`
	EnvironmentAllowlist []string `json:"environment_allowlist,omitempty"`
	NetworkPolicy        string   `json:"network_policy,omitempty"`
	UserPresence         string   `json:"user_presence,omitempty"`
	Foreground           string   `json:"foreground,omitempty"`
	InteractionMode      string   `json:"interaction_mode,omitempty"`
	MaxConcurrency       int      `json:"max_concurrency,omitempty"`
}

type Budget struct {
	MaxInputTokens   int64         `json:"max_input_tokens,omitempty"`
	MaxOutputTokens  int64         `json:"max_output_tokens,omitempty"`
	MaxWallTime      time.Duration `json:"max_wall_time,omitempty"`
	MaxSteps         int           `json:"max_steps,omitempty"`
	MaxToolActions   int           `json:"max_tool_actions,omitempty"`
	MaxCost          string        `json:"max_cost,omitempty"`
	Currency         string        `json:"currency,omitempty"`
	SubscriptionUnit string        `json:"subscription_unit,omitempty"`
}

type DegradationDefault string

const (
	DegradationDefaultReject        DegradationDefault = "reject"
	DegradationDefaultAllowReported DegradationDefault = "allow_reported"
)

type DegradationPolicy struct {
	Default             DegradationDefault `json:"default"`
	AllowedPaths        []string           `json:"allowed_paths,omitempty"`
	AllowedFidelities   []SemanticFidelity `json:"allowed_fidelities,omitempty"`
	ForbiddenActions    []string           `json:"forbidden_actions,omitempty"`
	RequirePreflightAck bool               `json:"require_preflight_ack,omitempty"`
	RequireExplanation  bool               `json:"require_explanation,omitempty"`
}

type IntentKind string

const (
	IntentCreateFile        IntentKind = "create_file"
	IntentModifyFile        IntentKind = "modify_file"
	IntentRewriteFile       IntentKind = "rewrite_file"
	IntentDeleteFile        IntentKind = "delete_file"
	IntentMoveFile          IntentKind = "move_file"
	IntentCreateDirectory   IntentKind = "create_directory"
	IntentDeleteDirectory   IntentKind = "delete_directory"
	IntentProduceStructured IntentKind = "produce_structured_output"
	IntentCallTool          IntentKind = "call_tool"
	IntentExecuteCode       IntentKind = "execute_code"
	IntentComputerUse       IntentKind = "computer_use"
)

type Condition struct {
	Kind    string          `json:"kind"`
	Target  string          `json:"target,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type IntentNode struct {
	ID               IntentID           `json:"id"`
	Kind             IntentKind         `json:"kind"`
	Target           string             `json:"target"`
	Specification    json.RawMessage    `json:"specification,omitempty"`
	Preconditions    []Condition        `json:"preconditions,omitempty"`
	Postconditions   []Condition        `json:"postconditions,omitempty"`
	DependsOn        []IntentID         `json:"depends_on,omitempty"`
	AtomicGroup      string             `json:"atomic_group,omitempty"`
	Required         bool               `json:"required"`
	Idempotency      string             `json:"idempotency,omitempty"`
	ConflictPolicy   string             `json:"conflict_policy,omitempty"`
	AcceptedFidelity []SemanticFidelity `json:"accepted_fidelity,omitempty"`
	Metadata         map[string]string  `json:"metadata,omitempty"`
}

type IntentGraph struct {
	Nodes []IntentNode `json:"nodes"`
}

type UnifiedExecutionRequest struct {
	SemanticVersion   string                     `json:"semantic_version"`
	ExecutionID       ExecutionID                `json:"execution_id"`
	ProfileSelector   ProfileSelector            `json:"profile_selector"`
	ExecutionKind     ExecutionKind              `json:"execution_kind"`
	Input             []InputItem                `json:"input,omitempty"`
	Instructions      []Instruction              `json:"instructions,omitempty"`
	Context           []ContextReference         `json:"context,omitempty"`
	Tools             []ToolDefinition           `json:"tools,omitempty"`
	ToolPolicy        ToolPolicy                 `json:"tool_policy"`
	OutputContract    OutputContract             `json:"output_contract"`
	ReasoningIntent   ReasoningIntent            `json:"reasoning_intent"`
	SessionIntent     SessionIntent              `json:"session_intent"`
	ExecutionPolicy   ExecutionPolicy            `json:"execution_policy"`
	Budget            Budget                     `json:"budget"`
	DegradationPolicy DegradationPolicy          `json:"degradation_policy"`
	IntentGraph       IntentGraph                `json:"intent_graph"`
	Metadata          map[string]string          `json:"metadata,omitempty"`
	Extensions        map[string]json.RawMessage `json:"extensions,omitempty"`
}

type MechanismPlan struct {
	ID                 MechanismPlanID    `json:"id"`
	IntentID           IntentID           `json:"intent_id"`
	Kind               string             `json:"kind"`
	Origin             CapabilityOrigin   `json:"origin"`
	Owner              ExecutionOwner     `json:"owner"`
	SelectionAuthority SelectionAuthority `json:"selection_authority"`
	CapabilityRef      string             `json:"capability_ref,omitempty"`
	PreferredRank      int                `json:"preferred_rank,omitempty"`
	HardConstraints    []string           `json:"hard_constraints,omitempty"`
	ExpectedEffects    []string           `json:"expected_effects,omitempty"`
	VerificationPlanID VerificationID     `json:"verification_plan_id,omitempty"`
	FallbackPlanIDs    []MechanismPlanID  `json:"fallback_plan_ids,omitempty"`
	SemanticFidelity   SemanticFidelity   `json:"semantic_fidelity"`
}

type MechanismAttempt struct {
	ID                 MechanismAttemptID `json:"id"`
	MechanismPlanID    MechanismPlanID    `json:"mechanism_plan_id"`
	RetryOf            MechanismAttemptID `json:"retry_of,omitempty"`
	SupersededBy       MechanismAttemptID `json:"superseded_by,omitempty"`
	Authoritative      bool               `json:"authoritative"`
	ActualKind         string             `json:"actual_kind"`
	ActualOrigin       CapabilityOrigin   `json:"actual_origin"`
	ActualOwner        ExecutionOwner     `json:"actual_owner"`
	NativeToolIdentity *NativeIdentity    `json:"native_tool_identity,omitempty"`
	StartedAt          time.Time          `json:"started_at,omitempty"`
	EndedAt            time.Time          `json:"ended_at,omitempty"`
	Status             AttemptStatus      `json:"status"`
	SanitizedInput     json.RawMessage    `json:"sanitized_input,omitempty"`
	OutputRefs         []string           `json:"output_refs,omitempty"`
	FailureClass       string             `json:"failure_class,omitempty"`
	SideEffectState    SideEffectState    `json:"side_effect_state"`
}

type EvidenceRef struct {
	Kind        string    `json:"kind"`
	Source      string    `json:"source"`
	Digest      string    `json:"digest"`
	CapturedAt  time.Time `json:"captured_at"`
	Sensitivity string    `json:"sensitivity"`
}

type FileStateType string

const (
	FileStateAbsent    FileStateType = "absent"
	FileStateRegular   FileStateType = "regular"
	FileStateDirectory FileStateType = "directory"
	FileStateSymlink   FileStateType = "symlink"
	FileStateOther     FileStateType = "other"
)

type FileStateSnapshot struct {
	Path       string        `json:"path"`
	Exists     bool          `json:"exists"`
	Type       FileStateType `json:"type,omitempty"`
	Hash       string        `json:"hash,omitempty"`
	Size       int64         `json:"size,omitempty"`
	Mode       uint32        `json:"mode,omitempty"`
	ModifiedAt time.Time     `json:"modified_at,omitempty"`
	Symlink    string        `json:"symlink,omitempty"`
}

type WorkspaceChange struct {
	Kind              string             `json:"kind"`
	Path              string             `json:"path"`
	Destination       string             `json:"destination,omitempty"`
	Before            *FileStateSnapshot `json:"before,omitempty"`
	After             *FileStateSnapshot `json:"after,omitempty"`
	DestinationBefore *FileStateSnapshot `json:"destination_before,omitempty"`
	DestinationAfter  *FileStateSnapshot `json:"destination_after,omitempty"`
	UnifiedDiff       string             `json:"unified_diff,omitempty"`
}

type StructuredOutputMechanism string

const (
	StructuredStrictJSONSchema StructuredOutputMechanism = "strict_json_schema"
	StructuredHarnessSchema    StructuredOutputMechanism = "harness_output_schema"
	StructuredToolSchema       StructuredOutputMechanism = "tool_schema"
	StructuredJSONObject       StructuredOutputMechanism = "json_object"
	StructuredEmulatedSchema   StructuredOutputMechanism = "emulated_strict_schema"
	StructuredPromptedJSON     StructuredOutputMechanism = "prompted_json"
)

type StructuredOutputEffect struct {
	Mechanism      StructuredOutputMechanism `json:"mechanism"`
	Origin         CapabilityOrigin          `json:"origin"`
	Fidelity       SemanticFidelity          `json:"fidelity"`
	Transport      string                    `json:"transport,omitempty"`
	RawRef         string                    `json:"raw_ref,omitempty"`
	Parsed         json.RawMessage           `json:"parsed,omitempty"`
	SchemaDigest   string                    `json:"schema_digest"`
	JSONValid      bool                      `json:"json_valid"`
	SchemaValid    bool                      `json:"schema_valid"`
	RepairAttempts int                       `json:"repair_attempts,omitempty"`
	FinalDigest    string                    `json:"final_digest,omitempty"`
}

type CodeExecutionEffect struct {
	Mechanism              string           `json:"mechanism"`
	Origin                 CapabilityOrigin `json:"origin"`
	Argv                   []string         `json:"argv"`
	RuntimeIdentity        string           `json:"runtime_identity"`
	EnvironmentFingerprint string           `json:"environment_fingerprint,omitempty"`
	ExitCode               *int             `json:"exit_code,omitempty"`
	StdoutRef              string           `json:"stdout_ref,omitempty"`
	StderrRef              string           `json:"stderr_ref,omitempty"`
	Duration               time.Duration    `json:"duration,omitempty"`
	NetworkEvidence        []EvidenceRef    `json:"network_evidence,omitempty"`
}

type ToolCallEffect struct {
	ToolID          string           `json:"tool_id"`
	ActionID        ActionID         `json:"action_id"`
	Mechanism       string           `json:"mechanism"`
	Origin          CapabilityOrigin `json:"origin"`
	Owner           ExecutionOwner   `json:"owner"`
	Executed        bool             `json:"executed"`
	InputDigest     string           `json:"input_digest"`
	OutputDigest    string           `json:"output_digest,omitempty"`
	ResultOrigin    EventOrigin      `json:"result_origin"`
	SideEffectState SideEffectState  `json:"side_effect_state"`
}

type ComputerUseEffect struct {
	Mechanism        string           `json:"mechanism"`
	Origin           CapabilityOrigin `json:"origin"`
	Action           string           `json:"action"`
	Target           string           `json:"target,omitempty"`
	BeforeRefs       []EvidenceRef    `json:"before_refs,omitempty"`
	AfterRefs        []EvidenceRef    `json:"after_refs,omitempty"`
	ExternalReadback string           `json:"external_readback,omitempty"`
}

type EffectPayload struct {
	WorkspaceChange  *WorkspaceChange        `json:"workspace_change,omitempty"`
	StructuredOutput *StructuredOutputEffect `json:"structured_output,omitempty"`
	ToolCall         *ToolCallEffect         `json:"tool_call,omitempty"`
	CodeExecution    *CodeExecutionEffect    `json:"code_execution,omitempty"`
	ComputerUse      *ComputerUseEffect      `json:"computer_use,omitempty"`
	Extension        json.RawMessage         `json:"extension,omitempty"`
}

type EffectRecord struct {
	ID                  EffectID           `json:"id"`
	IntentIDs           []IntentID         `json:"intent_ids"`
	MechanismAttemptID  MechanismAttemptID `json:"mechanism_attempt_id"`
	Kind                string             `json:"kind"`
	Target              string             `json:"target"`
	Payload             EffectPayload      `json:"payload"`
	EvidenceRefs        []EvidenceRef      `json:"evidence_refs,omitempty"`
	ObservationSource   string             `json:"observation_source"`
	VerificationStatus  VerificationStatus `json:"verification_status"`
	VerificationRefs    []VerificationID   `json:"verification_refs,omitempty"`
	SupersedesEffectIDs []EffectID         `json:"supersedes_effect_ids,omitempty"`
	Confidence          string             `json:"confidence,omitempty"`
	OccurredAt          time.Time          `json:"occurred_at"`
}

type VerificationRecord struct {
	ID           VerificationID     `json:"id"`
	EffectIDs    []EffectID         `json:"effect_ids,omitempty"`
	IntentIDs    []IntentID         `json:"intent_ids,omitempty"`
	Kind         string             `json:"kind"`
	Status       VerificationStatus `json:"status"`
	Verifier     VersionedIdentity  `json:"verifier"`
	EvidenceRefs []EvidenceRef      `json:"evidence_refs,omitempty"`
	FailureCode  string             `json:"failure_code,omitempty"`
	CompletedAt  time.Time          `json:"completed_at,omitempty"`
}

type IntentSatisfaction struct {
	IntentID              IntentID                 `json:"intent_id"`
	Status                IntentSatisfactionStatus `json:"status"`
	EffectIDs             []EffectID               `json:"effect_ids,omitempty"`
	MissingPostconditions []string                 `json:"missing_postconditions,omitempty"`
	Residuals             []string                 `json:"residuals,omitempty"`
}

type ToolProbeStatus string

const (
	ToolProbeNotRun   ToolProbeStatus = "not_run"
	ToolProbeReported ToolProbeStatus = "reported"
	ToolProbeObserved ToolProbeStatus = "observed"
)

type ToolSurfaceProbe struct {
	Status         ToolProbeStatus `json:"status"`
	EvidenceDigest string          `json:"evidence_digest,omitempty"`
	ObservedAt     time.Time       `json:"observed_at,omitempty"`
}

type ToolSurfaceEntry struct {
	ID             string           `json:"id"`
	NativeName     string           `json:"native_name,omitempty"`
	Discovered     bool             `json:"discovered"`
	Registered     bool             `json:"registered"`
	ModelVisible   bool             `json:"model_visible"`
	Executable     bool             `json:"executable"`
	PermissionMode string           `json:"permission_mode"`
	AutoApproved   bool             `json:"auto_approved"`
	Owner          ExecutionOwner   `json:"owner"`
	FallbackOwner  ExecutionOwner   `json:"fallback_owner,omitempty"`
	SchemaDigest   string           `json:"schema_digest,omitempty"`
	Probe          ToolSurfaceProbe `json:"probe"`
}

type ToolSurfaceManifest struct {
	Entries []ToolSurfaceEntry `json:"entries,omitempty"`
}

type ManifestComponent struct {
	Kind         string         `json:"kind"`
	Name         string         `json:"name"`
	Version      string         `json:"version,omitempty"`
	State        string         `json:"state"`
	Digest       string         `json:"digest,omitempty"`
	Owner        ExecutionOwner `json:"owner,omitempty"`
	ModelVisible bool           `json:"model_visible,omitempty"`
	Executable   bool           `json:"executable,omitempty"`
	Opaque       bool           `json:"opaque,omitempty"`
}

type ContextManifestSummary struct {
	ID           string              `json:"id"`
	Version      string              `json:"version"`
	Mode         string              `json:"mode"`
	Components   []ManifestComponent `json:"components,omitempty"`
	Tools        ToolSurfaceManifest `json:"tools"`
	OpaqueFields []string            `json:"opaque_fields,omitempty"`
	Digest       string              `json:"digest,omitempty"`
}

type Residual struct {
	Path       string `json:"path"`
	Capability string `json:"capability,omitempty"`
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Impact     string `json:"impact"`
	Mitigation string `json:"mitigation,omitempty"`
}

type MappingDecision struct {
	Path     string           `json:"path"`
	Fidelity SemanticFidelity `json:"fidelity"`
	Origin   CapabilityOrigin `json:"origin"`
	Detail   string           `json:"detail,omitempty"`
}

type MappingReport struct {
	Decisions []MappingDecision `json:"decisions,omitempty"`
	Digest    string            `json:"digest,omitempty"`
}

type PreparedExecutionPlan struct {
	SemanticVersion  string                 `json:"semantic_version"`
	ExecutionID      ExecutionID            `json:"execution_id"`
	Profile          VersionedIdentity      `json:"profile"`
	Route            VersionedIdentity      `json:"route"`
	ProfileKeyDigest string                 `json:"profile_key_digest"`
	ExecutionKind    ExecutionKind          `json:"execution_kind"`
	IntentGraph      IntentGraph            `json:"intent_graph"`
	Mechanisms       []MechanismPlan        `json:"mechanisms"`
	ExpectedManifest ContextManifestSummary `json:"expected_manifest"`
	Residuals        []Residual             `json:"residuals,omitempty"`
	MappingReport    MappingReport          `json:"mapping_report"`
	RouteFingerprint string                 `json:"route_fingerprint"`
	Digest           string                 `json:"digest,omitempty"`
	Metadata         map[string]string      `json:"metadata,omitempty"`
}

type EventHeader struct {
	EventID                EventID                `json:"event_id"`
	SemanticVersion        string                 `json:"semantic_version"`
	ExecutionID            ExecutionID            `json:"execution_id"`
	SessionID              SessionID              `json:"session_id,omitempty"`
	TurnID                 TurnID                 `json:"turn_id,omitempty"`
	ItemID                 ItemID                 `json:"item_id,omitempty"`
	ParentID               ItemID                 `json:"parent_id,omitempty"`
	ActionID               ActionID               `json:"action_id,omitempty"`
	IntentID               IntentID               `json:"intent_id,omitempty"`
	MechanismPlanID        MechanismPlanID        `json:"mechanism_plan_id,omitempty"`
	MechanismAttemptID     MechanismAttemptID     `json:"mechanism_attempt_id,omitempty"`
	EffectID               EffectID               `json:"effect_id,omitempty"`
	VerificationID         VerificationID         `json:"verification_id,omitempty"`
	ApprovalID             ApprovalID             `json:"approval_id,omitempty"`
	Sequence               uint64                 `json:"sequence"`
	SourceSequence         uint64                 `json:"source_sequence,omitempty"`
	Timestamp              time.Time              `json:"timestamp"`
	SourceTimestamp        time.Time              `json:"source_timestamp,omitempty"`
	IngestedAt             time.Time              `json:"ingested_at,omitempty"`
	CausationID            EventID                `json:"causation_id,omitempty"`
	CorrelationID          string                 `json:"correlation_id,omitempty"`
	Origin                 EventOrigin            `json:"origin"`
	Family                 EventFamily            `json:"family"`
	Visibility             Visibility             `json:"visibility"`
	SecurityClassification SecurityClassification `json:"security_classification"`
	ExecutionKind          ExecutionKind          `json:"execution_kind"`
	Profile                VersionedIdentity      `json:"profile"`
	Route                  VersionedIdentity      `json:"route"`
	NativeIdentity         *NativeIdentity        `json:"native_identity,omitempty"`
}

type LifecycleEvent struct {
	Kind                  string          `json:"kind"`
	Status                ExecutionStatus `json:"status,omitempty"`
	StopReason            string          `json:"stop_reason,omitempty"`
	PendingBackgroundWork int             `json:"pending_background_work,omitempty"`
}

type IntentEvent struct {
	Kind         string              `json:"kind"`
	Satisfaction *IntentSatisfaction `json:"satisfaction,omitempty"`
}

type MechanismEvent struct {
	Kind    string            `json:"kind"`
	Plan    *MechanismPlan    `json:"plan,omitempty"`
	Attempt *MechanismAttempt `json:"attempt,omitempty"`
}

type ModelEvent struct {
	Kind            string          `json:"kind"`
	Content         []ContentPart   `json:"content,omitempty"`
	ActionID        ActionID        `json:"action_id,omitempty"`
	ResultOrigin    EventOrigin     `json:"result_origin,omitempty"`
	ExecutionItemID ItemID          `json:"execution_item_id,omitempty"`
	Executed        *bool           `json:"executed,omitempty"`
	SyntheticReason string          `json:"synthetic_reason,omitempty"`
	DisclosureClass string          `json:"disclosure_class,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Usage           []UsageMetric   `json:"usage,omitempty"`
}

type ExecutionItem struct {
	ID              ItemID             `json:"id"`
	Kind            string             `json:"kind"`
	Status          ItemStatus         `json:"status"`
	ActionID        ActionID           `json:"action_id,omitempty"`
	AttemptID       MechanismAttemptID `json:"attempt_id,omitempty"`
	SideEffectState SideEffectState    `json:"side_effect_state"`
	Payload         json.RawMessage    `json:"payload,omitempty"`
}

type ItemEvent struct {
	Kind  string          `json:"kind"`
	Item  ExecutionItem   `json:"item"`
	Delta json.RawMessage `json:"delta,omitempty"`
}

type EffectEvent struct {
	Kind         string              `json:"kind"`
	Effect       *EffectRecord       `json:"effect,omitempty"`
	Verification *VerificationRecord `json:"verification,omitempty"`
}

type ControlEvent struct {
	Kind                  string             `json:"kind"`
	ApprovalID            ApprovalID         `json:"approval_id,omitempty"`
	ActionID              ActionID           `json:"action_id,omitempty"`
	MechanismAttemptID    MechanismAttemptID `json:"mechanism_attempt_id,omitempty"`
	InputDigest           string             `json:"input_digest,omitempty"`
	ActionRevision        uint64             `json:"action_revision,omitempty"`
	Scope                 string             `json:"scope,omitempty"`
	Authority             string             `json:"authority,omitempty"`
	ExpiresAt             time.Time          `json:"expires_at,omitempty"`
	IdempotencyKey        string             `json:"idempotency_key,omitempty"`
	Decision              string             `json:"decision,omitempty"`
	PendingBackgroundWork int                `json:"pending_background_work,omitempty"`
	Payload               json.RawMessage    `json:"payload,omitempty"`
}

type DiagnosticEvent struct {
	Kind     string                  `json:"kind"`
	Code     string                  `json:"code,omitempty"`
	Message  string                  `json:"message,omitempty"`
	Residual *Residual               `json:"residual,omitempty"`
	Manifest *ContextManifestSummary `json:"manifest,omitempty"`
	Payload  json.RawMessage         `json:"payload,omitempty"`
}

type UnifiedExecutionEvent struct {
	Header     EventHeader      `json:"header"`
	Lifecycle  *LifecycleEvent  `json:"lifecycle,omitempty"`
	Intent     *IntentEvent     `json:"intent,omitempty"`
	Mechanism  *MechanismEvent  `json:"mechanism,omitempty"`
	Model      *ModelEvent      `json:"model,omitempty"`
	Item       *ItemEvent       `json:"item,omitempty"`
	Effect     *EffectEvent     `json:"effect,omitempty"`
	Control    *ControlEvent    `json:"control,omitempty"`
	Diagnostic *DiagnosticEvent `json:"diagnostic,omitempty"`
}

type ExecutionCommand struct {
	SemanticVersion         string             `json:"semantic_version"`
	ExecutionID             ExecutionID        `json:"execution_id"`
	SessionID               SessionID          `json:"session_id,omitempty"`
	TurnID                  TurnID             `json:"turn_id,omitempty"`
	Kind                    CommandKind        `json:"kind"`
	ExpectedExecutionStatus string             `json:"expected_execution_status"`
	IdempotencyKey          string             `json:"idempotency_key"`
	ApprovalID              ApprovalID         `json:"approval_id,omitempty"`
	ActionID                ActionID           `json:"action_id,omitempty"`
	MechanismAttemptID      MechanismAttemptID `json:"mechanism_attempt_id,omitempty"`
	InputDigest             string             `json:"input_digest,omitempty"`
	ActionRevision          uint64             `json:"action_revision,omitempty"`
	Payload                 json.RawMessage    `json:"payload,omitempty"`
}

type UsageMetric struct {
	Kind    string  `json:"kind"`
	Value   float64 `json:"value,omitempty"`
	Unit    string  `json:"unit"`
	Scope   string  `json:"scope"`
	Source  string  `json:"source"`
	Quality string  `json:"quality"`
}

type UnifiedError struct {
	Kind      string `json:"kind"`
	Phase     string `json:"phase"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

type UnifiedExecutionResult struct {
	SemanticVersion       string                 `json:"semantic_version"`
	ExecutionID           ExecutionID            `json:"execution_id"`
	SessionID             SessionID              `json:"session_id,omitempty"`
	TurnID                TurnID                 `json:"turn_id,omitempty"`
	TerminalEventID       EventID                `json:"terminal_event_id"`
	Status                ExecutionStatus        `json:"status"`
	VerificationStatus    VerificationStatus     `json:"verification_status"`
	StopReason            string                 `json:"stop_reason,omitempty"`
	IntentSatisfaction    []IntentSatisfaction   `json:"intent_satisfaction,omitempty"`
	MechanismTrace        []MechanismAttempt     `json:"mechanism_trace,omitempty"`
	Effects               []EffectRecord         `json:"effects,omitempty"`
	Verifications         []VerificationRecord   `json:"verifications,omitempty"`
	FinalContent          []ContentPart          `json:"final_content,omitempty"`
	Actions               []ExecutionItem        `json:"actions,omitempty"`
	Artifacts             []ArtifactID           `json:"artifacts,omitempty"`
	WorkspaceChanges      []WorkspaceChange      `json:"workspace_changes,omitempty"`
	UsageMetrics          []UsageMetric          `json:"usage_metrics,omitempty"`
	MappingReport         MappingReport          `json:"mapping_report"`
	ContextManifest       ContextManifestSummary `json:"context_manifest"`
	Residuals             []Residual             `json:"residuals,omitempty"`
	PendingBackgroundWork int                    `json:"pending_background_work"`
	Error                 *UnifiedError          `json:"error,omitempty"`
	Digest                string                 `json:"digest,omitempty"`
}
