package applicationadapter

import (
	"testing"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
)

func TestPhaseDecisionV1ClosedMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		state    contract.CaseStateV1
		verdict  *contract.VerdictV1
		expected applicationcontract.ReviewPhaseDecisionV1
	}{
		{"pending", contract.CaseRoutedV1, nil, applicationcontract.ReviewPhaseDeferV1},
		{"accepted", contract.CaseResolvedV1, &contract.VerdictV1{State: contract.VerdictAcceptedV1}, applicationcontract.ReviewPhaseAllowV1},
		{"rejected", contract.CaseResolvedV1, &contract.VerdictV1{State: contract.VerdictRejectedV1}, applicationcontract.ReviewPhaseDenyV1},
		{"conditional", contract.CaseResolvedV1, &contract.VerdictV1{State: contract.VerdictConditionalV1}, applicationcontract.ReviewPhaseDeferV1},
		{"waiting revision", contract.CaseWaitingRevisionV1, nil, applicationcontract.ReviewPhaseAskV1},
		{"waiting human", contract.CaseWaitingHumanV1, nil, applicationcontract.ReviewPhaseAskV1},
		{"waiting evidence", contract.CaseWaitingEvidenceV1, nil, applicationcontract.ReviewPhaseAskV1},
		{"expired", contract.CaseExpiredV1, nil, applicationcontract.ReviewPhaseDenyV1},
		{"revoked", contract.CaseRevokedV1, nil, applicationcontract.ReviewPhaseDenyV1},
		{"superseded", contract.CaseSupersededV1, nil, applicationcontract.ReviewPhaseDenyV1},
		{"cancelled", contract.CaseCancelledV1, nil, applicationcontract.ReviewPhaseDenyV1},
	}
	for _, test := range tests {
		if got := phaseDecisionV1(contract.ReviewCaseV1{State: test.state}, test.verdict); got != test.expected {
			t.Errorf("%s: got %s want %s", test.name, got, test.expected)
		}
	}
}
