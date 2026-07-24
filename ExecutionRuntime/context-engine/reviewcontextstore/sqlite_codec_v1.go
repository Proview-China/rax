package reviewcontextstore

import (
	"encoding/json"

	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const reviewerContextSQLiteCanonicalDomainV1 = "praxis.context.reviewer-context.sqlite"

func encodeReviewerContextRowV1(value reviewcontract.ReviewerContextEnvelopeV1) ([]byte, core.Digest, error) {
	payload, err := json.Marshal(value.Clone())
	if err != nil || len(payload) == 0 || len(payload) > core.MaxCanonicalDocumentBytes {
		return nil, "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Reviewer Context sqlite row is invalid or too large")
	}
	digest, err := core.CanonicalJSONDigest(reviewerContextSQLiteCanonicalDomainV1, "v1", "ReviewerContextEnvelopeV1", value.Clone())
	if err != nil {
		return nil, "", err
	}
	return payload, digest, nil
}

func decodeReviewerContextRowV1(payload []byte, storedRowDigest string) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	var value reviewcontract.ReviewerContextEnvelopeV1
	if err := core.DecodeStrictJSON(payload, &value); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Reviewer Context sqlite row is not strict canonical JSON")
	}
	digest, err := core.CanonicalJSONDigest(reviewerContextSQLiteCanonicalDomainV1, "v1", "ReviewerContextEnvelopeV1", value.Clone())
	if err != nil || string(digest) != storedRowDigest {
		return reviewcontract.ReviewerContextEnvelopeV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Reviewer Context sqlite row digest drifted")
	}
	if err := value.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Reviewer Context sqlite envelope validation drifted")
	}
	return value.Clone(), nil
}
