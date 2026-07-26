package conformance

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

const BackendSuiteContractVersionV1 = "praxis.continuity.backend-conformance/v1"

var backendChecksV1 = []string{
	"content.missing-put-get-clone",
	"content.digest-conflict",
	"metadata.manifest-stage-inspect-clone",
	"metadata.manifest-conflict-commit-visible",
	"metadata.journal-create-inspect",
	"metadata.journal-identity-conflict",
	"metadata.journal-cas-stale",
	"metadata.journal-concurrent-single-winner",
	"retention.create-inspect",
	"retention.changed-create-conflict",
	"retention.cas-stale",
}

// BackendSuiteRequestV1 supplies isolated backend instances to the reusable
// reference conformance suite. Namespace must be fresh for the supplied stores:
// the suite intentionally mutates only objects derived from that namespace.
type BackendSuiteRequestV1 struct {
	Namespace   string
	NowUnixNano int64
	Metadata    ports.MetadataStore
	Content     ports.ContentStore
	Retention   ports.RetentionStore
}

// BackendSuiteReportV1 is test evidence only. It is deliberately incapable of
// certifying a production backend, provider, deployment, or SLA.
type BackendSuiteReportV1 struct {
	ContractVersion    string   `json:"contract_version"`
	Namespace          string   `json:"namespace"`
	ReferenceOnly      bool     `json:"reference_only"`
	ProductionEligible bool     `json:"production_eligible"`
	Checks             []string `json:"checks"`
}

func (r BackendSuiteReportV1) Validate() error {
	if r.ContractVersion != BackendSuiteContractVersionV1 {
		return contract.NewError(contract.ErrInvalidArgument, "contract_version", "backend conformance report version differs")
	}
	if err := validateBackendNamespaceV1(r.Namespace); err != nil {
		return err
	}
	if !r.ReferenceOnly || r.ProductionEligible {
		return contract.NewError(contract.ErrPreconditionFailed, "production_eligible", "backend suite cannot certify production")
	}
	return validateExactSet("checks", r.Checks, backendChecksV1)
}

func (r BackendSuiteReportV1) Clone() BackendSuiteReportV1 {
	r.Checks = append([]string(nil), r.Checks...)
	return r
}

// RunBackendSuiteV1 checks the existing backend-neutral Metadata, Content, and
// Retention SPI semantics. It does not inspect deployment, encryption/KMS,
// remote durability, backup, Participant behavior, or production readiness.
func RunBackendSuiteV1(ctx context.Context, request BackendSuiteRequestV1) (BackendSuiteReportV1, error) {
	if ctx == nil {
		return BackendSuiteReportV1{}, contract.NewError(contract.ErrInvalidArgument, "context", "context is required")
	}
	if err := ctx.Err(); err != nil {
		return BackendSuiteReportV1{}, err
	}
	if err := validateBackendNamespaceV1(request.Namespace); err != nil {
		return BackendSuiteReportV1{}, err
	}
	if request.NowUnixNano <= 0 || request.NowUnixNano == math.MaxInt64 {
		return BackendSuiteReportV1{}, contract.NewError(contract.ErrInvalidArgument, "now_unix_nano", "bounded positive deterministic time is required")
	}
	if nilBackendV1(request.Metadata) || nilBackendV1(request.Content) || nilBackendV1(request.Retention) {
		return BackendSuiteReportV1{}, contract.NewError(contract.ErrInvalidArgument, "backend", "metadata, content, and retention stores are required")
	}
	if err := checkContentV1(ctx, request); err != nil {
		return BackendSuiteReportV1{}, err
	}
	manifest, err := checkManifestV1(ctx, request)
	if err != nil {
		return BackendSuiteReportV1{}, err
	}
	if err := checkJournalV1(ctx, request, manifest); err != nil {
		return BackendSuiteReportV1{}, err
	}
	if err := checkRetentionV1(ctx, request, manifest.ObjectID); err != nil {
		return BackendSuiteReportV1{}, err
	}
	report := BackendSuiteReportV1{
		ContractVersion: BackendSuiteContractVersionV1,
		Namespace:       request.Namespace, ReferenceOnly: true,
		ProductionEligible: false,
		Checks:             append([]string(nil), backendChecksV1...),
	}
	return report, report.Validate()
}

func checkContentV1(ctx context.Context, request BackendSuiteRequestV1) error {
	data := []byte("praxis-continuity-backend-conformance:" + request.Namespace)
	ref := contract.ChunkRef{SchemaVersion: "chunk/v1", Digest: contract.DigestBytes(data), Length: int64(len(data))}
	has, err := request.Content.HasChunk(ctx, ref)
	if err != nil || has {
		return backendFailureV1("content_missing", "fresh namespace must not contain the probe chunk", err)
	}
	input := append([]byte(nil), data...)
	if err := request.Content.PutChunk(ctx, ref, input); err != nil {
		return backendFailureV1("content_put", "valid chunk put failed", err)
	}
	input[0] ^= 0xff
	if err := request.Content.PutChunk(ctx, ref, data); err != nil {
		return backendFailureV1("content_idempotency", "exact chunk replay failed", err)
	}
	has, err = request.Content.HasChunk(ctx, ref)
	if err != nil || !has {
		return backendFailureV1("content_has", "stored chunk is not reported present", err)
	}
	got, err := request.Content.GetChunk(ctx, ref)
	if err != nil || !bytes.Equal(got, data) {
		return backendFailureV1("content_get", "stored chunk differs", err)
	}
	got[0] ^= 0xff
	again, err := request.Content.GetChunk(ctx, ref)
	if err != nil || !bytes.Equal(again, data) {
		return backendFailureV1("content_clone", "returned chunk aliases backend state", err)
	}
	drift := append([]byte(nil), data...)
	drift[0] ^= 0xff
	if err := request.Content.PutChunk(ctx, ref, drift); !contract.HasCode(err, contract.ErrContentDigestMismatch) {
		return backendFailureV1("content_digest", "same ref accepted different bytes", err)
	}
	return nil
}

func checkManifestV1(ctx context.Context, request BackendSuiteRequestV1) (contract.ObjectManifest, error) {
	data := []byte("praxis-continuity-backend-conformance:" + request.Namespace)
	chunk := contract.ChunkRef{SchemaVersion: "chunk/v1", Digest: contract.DigestBytes(data), Length: int64(len(data))}
	manifest := contract.ObjectManifest{
		ContractVersion: contract.ContractVersion,
		ObjectID:        request.Namespace + "-object", SchemaVersion: "object/v1",
		ContentDigest: chunk.Digest, TotalLength: chunk.Length, Chunks: []contract.ChunkRef{chunk},
		Compression: "none", Classification: "internal", OwnerID: "continuity-conformance",
		ScopeDigest: request.Namespace + "-scope", RetentionPolicyRef: request.Namespace + "-policy",
		CreatedUnixNano: request.NowUnixNano,
	}
	digest, err := manifest.CanonicalDigest()
	if err != nil {
		return contract.ObjectManifest{}, err
	}
	manifest.Digest = digest
	staged := manifest
	staged.Chunks = append([]contract.ChunkRef(nil), manifest.Chunks...)
	if err := request.Metadata.StageManifest(ctx, staged); err != nil {
		return contract.ObjectManifest{}, backendFailureV1("manifest_stage", "valid manifest stage failed", err)
	}
	staged.Chunks[0].Digest = contract.DigestBytes([]byte("input-alias-drift"))
	if err := request.Metadata.StageManifest(ctx, manifest); err != nil {
		return contract.ObjectManifest{}, backendFailureV1("manifest_idempotency", "exact manifest stage replay failed", err)
	}
	got, visible, err := request.Metadata.InspectObject(ctx, manifest.ObjectID)
	if err != nil || visible || !reflect.DeepEqual(got, manifest) {
		return contract.ObjectManifest{}, backendFailureV1("manifest_inspect", "staged manifest inspection differs", err)
	}
	got.Chunks[0].Digest = contract.DigestBytes([]byte("alias-drift"))
	again, _, err := request.Metadata.InspectObject(ctx, manifest.ObjectID)
	if err != nil || !reflect.DeepEqual(again, manifest) {
		return contract.ObjectManifest{}, backendFailureV1("manifest_clone", "returned manifest aliases backend state", err)
	}
	changed := manifest
	changed.RetentionPolicyRef = request.Namespace + "-other-policy"
	changed.Digest, err = changed.CanonicalDigest()
	if err != nil {
		return contract.ObjectManifest{}, err
	}
	if err := request.Metadata.StageManifest(ctx, changed); !contract.HasCode(err, contract.ErrRevisionConflict) {
		return contract.ObjectManifest{}, backendFailureV1("manifest_conflict", "same object ID accepted changed manifest", err)
	}
	if err := request.Metadata.CommitObjectReference(ctx, manifest.ObjectID, contract.DigestBytes([]byte("wrong"))); !contract.HasCode(err, contract.ErrContentDigestMismatch) {
		return contract.ObjectManifest{}, backendFailureV1("manifest_commit_digest", "wrong content digest was committed", err)
	}
	if err := request.Metadata.CommitObjectReference(ctx, manifest.ObjectID, manifest.ContentDigest); err != nil {
		return contract.ObjectManifest{}, backendFailureV1("manifest_commit", "exact object reference commit failed", err)
	}
	if err := request.Metadata.SetObjectVisible(ctx, manifest.ObjectID, true); err != nil {
		return contract.ObjectManifest{}, backendFailureV1("manifest_visible", "committed object did not become visible", err)
	}
	got, visible, err = request.Metadata.InspectObject(ctx, manifest.ObjectID)
	if err != nil || !visible || !reflect.DeepEqual(got, manifest) {
		return contract.ObjectManifest{}, backendFailureV1("manifest_visible_inspect", "visible manifest inspection differs", err)
	}
	return manifest, nil
}

func checkJournalV1(ctx context.Context, request BackendSuiteRequestV1, manifest contract.ObjectManifest) error {
	journal := contract.WriteJournal{
		JournalID: request.Namespace + "-journal", ObjectID: manifest.ObjectID,
		ObjectDigest: manifest.ContentDigest, ManifestDigest: manifest.Digest,
		State: contract.JournalProposed, Revision: 1,
		ResidualRefs: []contract.ResidualRef{{
			ID: request.Namespace + "-residual", Kind: "conformance-probe", OwnerID: "continuity-conformance",
			ScopeDigest: request.Namespace + "-scope", SubjectDigest: manifest.Digest,
			State: "open", ConflictDomain: request.Namespace + "-conflict",
		}},
		UpdatedUnixNano: request.NowUnixNano,
	}
	created := journal
	created.ResidualRefs = append([]contract.ResidualRef(nil), journal.ResidualRefs...)
	if err := request.Metadata.CreateJournal(ctx, created); err != nil {
		return backendFailureV1("journal_create", "valid journal create failed", err)
	}
	created.ResidualRefs[0].State = "input-alias-drift"
	if err := request.Metadata.CreateJournal(ctx, journal); err != nil {
		return backendFailureV1("journal_idempotency", "exact journal create replay failed", err)
	}
	got, err := request.Metadata.InspectJournal(ctx, journal.JournalID)
	if err != nil || !reflect.DeepEqual(got, journal) {
		return backendFailureV1("journal_inspect", "created journal inspection differs", err)
	}
	got.ResidualRefs[0].State = "output-alias-drift"
	again, err := request.Metadata.InspectJournal(ctx, journal.JournalID)
	if err != nil || !reflect.DeepEqual(again, journal) {
		return backendFailureV1("journal_clone", "returned journal aliases backend state", err)
	}
	changed := journal
	changed.ObjectDigest = contract.DigestBytes([]byte("changed-object"))
	if err := request.Metadata.CreateJournal(ctx, changed); !contract.HasCode(err, contract.ErrRevisionConflict) {
		return backendFailureV1("journal_identity", "same journal ID accepted changed immutable identity", err)
	}
	next := journal
	next.State = contract.JournalMetadataPending
	next.Revision = 2
	next.UpdatedUnixNano++
	if err := request.Metadata.CASJournal(ctx, 1, next); err != nil {
		return backendFailureV1("journal_cas", "valid journal CAS failed", err)
	}
	if err := request.Metadata.CASJournal(ctx, 1, next); !contract.HasCode(err, contract.ErrRevisionConflict) {
		return backendFailureV1("journal_stale", "stale journal CAS did not conflict", err)
	}
	got, err = request.Metadata.InspectJournal(ctx, journal.JournalID)
	if err != nil || !reflect.DeepEqual(got, next) {
		return backendFailureV1("journal_current", "journal CAS did not become current", err)
	}

	concurrent := journal
	concurrent.JournalID = request.Namespace + "-journal-concurrent"
	if err := request.Metadata.CreateJournal(ctx, concurrent); err != nil {
		return backendFailureV1("journal_concurrent_create", "concurrent probe create failed", err)
	}
	concurrentNext := concurrent
	concurrentNext.State = contract.JournalMetadataPending
	concurrentNext.Revision = 2
	concurrentNext.UpdatedUnixNano++
	var winners atomic.Int32
	var conflicts atomic.Int32
	var unexpected atomic.Int32
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := request.Metadata.CASJournal(ctx, 1, concurrentNext)
			switch {
			case err == nil:
				winners.Add(1)
			case contract.HasCode(err, contract.ErrRevisionConflict):
				conflicts.Add(1)
			default:
				unexpected.Add(1)
			}
		}()
	}
	wait.Wait()
	if winners.Load() != 1 || conflicts.Load() != 31 || unexpected.Load() != 0 {
		return backendFailureV1("journal_concurrent", fmt.Sprintf("CAS winners=%d conflicts=%d unexpected=%d", winners.Load(), conflicts.Load(), unexpected.Load()), nil)
	}
	got, err = request.Metadata.InspectJournal(ctx, concurrent.JournalID)
	if err != nil || !reflect.DeepEqual(got, concurrentNext) {
		return backendFailureV1("journal_concurrent_current", "concurrent winner did not become current", err)
	}
	return nil
}

func checkRetentionV1(ctx context.Context, request BackendSuiteRequestV1, objectID string) error {
	current := contract.RetentionFact{
		ObjectID: objectID, PolicyRef: request.Namespace + "-policy", Classification: "internal",
		State: contract.RetentionActive, Revision: 1, UpdatedUnixNano: request.NowUnixNano,
	}
	if err := request.Retention.CreateRetention(ctx, current); err != nil {
		return backendFailureV1("retention_create", "valid retention create failed", err)
	}
	got, err := request.Retention.InspectRetention(ctx, objectID)
	if err != nil || !reflect.DeepEqual(got, current) {
		return backendFailureV1("retention_inspect", "created retention inspection differs", err)
	}
	changed := current
	changed.PolicyRef = request.Namespace + "-other-policy"
	if err := request.Retention.CreateRetention(ctx, changed); !contract.HasCode(err, contract.ErrRevisionConflict) {
		return backendFailureV1("retention_identity", "same object accepted changed retention identity", err)
	}
	next, err := contract.AdvanceRetention(current, contract.RetentionExpired, request.Namespace+"-retention-evidence")
	if err != nil {
		return err
	}
	next.UpdatedUnixNano++
	if err := request.Retention.CASRetention(ctx, 1, next); err != nil {
		return backendFailureV1("retention_cas", "valid retention CAS failed", err)
	}
	if err := request.Retention.CASRetention(ctx, 1, next); !contract.HasCode(err, contract.ErrRevisionConflict) {
		return backendFailureV1("retention_stale", "stale retention CAS did not conflict", err)
	}
	got, err = request.Retention.InspectRetention(ctx, objectID)
	if err != nil || !reflect.DeepEqual(got, next) {
		return backendFailureV1("retention_current", "retention current inspection differs", err)
	}
	return nil
}

func validateBackendNamespaceV1(namespace string) error {
	if err := contract.ValidateToken("namespace", namespace); err != nil {
		return err
	}
	if len(namespace) > 128 {
		return contract.NewError(contract.ErrInvalidArgument, "namespace", "must leave room for deterministic probe suffixes")
	}
	return nil
}

func nilBackendV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func backendFailureV1(field, message string, cause error) error {
	if cause != nil {
		message += ": " + cause.Error()
	}
	return contract.NewError(contract.ErrPreconditionFailed, field, message)
}
