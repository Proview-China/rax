package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	reviewsdk "github.com/Proview-China/rax/ExecutionRuntime/review/sdk/go"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return usage()
	}
	base := os.Getenv("PRAXIS_REVIEW_URL")
	token := os.Getenv("PRAXIS_REVIEW_TOKEN")
	if base == "" || len(token) < 32 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "PRAXIS_REVIEW_URL and PRAXIS_REVIEW_TOKEN are required")
	}
	timeout := 30 * time.Second
	if raw := os.Getenv("PRAXIS_REVIEW_TIMEOUT"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 || value > 10*time.Minute {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "PRAXIS_REVIEW_TIMEOUT is invalid")
		}
		timeout = value
	}
	client, err := reviewsdk.New(reviewsdk.Config{BaseURL: base, HTTPClient: &http.Client{}, TokenProvider: reviewsdk.TokenProviderFuncV1(func(context.Context) (string, error) { return token, nil })})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	encode := func(value any) error {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	switch args[0] {
	case "submit":
		var command service.SubmitCommandV1
		if err := decodeInput(stdin, &command); err != nil {
			return err
		}
		value, err := client.SubmitV1(ctx, command)
		if err != nil {
			return err
		}
		return encode(value)
	case "list":
		fs := flag.NewFlagSet("list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		tenant := fs.String("tenant", "", "tenant ID")
		limit := fs.Int("limit", 50, "page size")
		cursor := fs.String("cursor", "", "sealed cursor")
		states := stringListFlag{}
		fs.Var(&states, "state", "case state (repeatable)")
		if err := fs.Parse(args[1:]); err != nil || *tenant == "" {
			return usage()
		}
		request := reviewsdk.ListRequestV1{TenantID: core.TenantID(*tenant), Limit: *limit, Cursor: *cursor}
		for _, state := range states {
			request.States = append(request.States, contract.CaseStateV1(state))
		}
		value, err := client.ListV1(ctx, request)
		if err != nil {
			return err
		}
		return encode(value)
	case "show":
		if len(args) != 3 {
			return usage()
		}
		value, err := client.GetV1(ctx, core.TenantID(args[1]), args[2])
		if err != nil {
			return err
		}
		return encode(value)
	case "watch":
		if len(args) < 3 || len(args) > 4 {
			return usage()
		}
		after := ""
		if len(args) == 4 {
			after = args[3]
		}
		_, err := client.WatchV1(parent, core.TenantID(args[1]), args[2], after, func(value contract.TraceFactV1) error { return encode(value) })
		return err
	case "events":
		fs := flag.NewFlagSet("events", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		tenant := fs.String("tenant", "", "tenant ID")
		caseID := fs.String("case", "", "case ID")
		limit := fs.Int("limit", 50, "page size")
		cursor := fs.String("cursor", "", "sealed cursor")
		if err := fs.Parse(args[1:]); err != nil || *tenant == "" || *caseID == "" || *limit <= 0 || *limit > reviewport.MaxTracePageV2 {
			return usage()
		}
		value, err := client.EventsPageV2(ctx, reviewsdk.EventsPageRequestV2{TenantID: core.TenantID(*tenant), CaseID: *caseID, Limit: *limit, Cursor: *cursor})
		if err != nil {
			return err
		}
		return encode(value)
	case "approve", "deny", "request-changes":
		var command reviewsdk.AttestCommandV1
		if err := decodeInput(stdin, &command); err != nil {
			return err
		}
		expected := map[string]contract.ResolutionV1{"approve": contract.ResolutionAcceptV1, "deny": contract.ResolutionRejectV1, "request-changes": contract.ResolutionRequestChangesV1}[args[0]]
		if command.Attestation.Resolution != expected {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "CLI command and sealed Attestation resolution drifted")
		}
		caseFact, attestation, err := client.AttestV1(ctx, command)
		if err != nil {
			return err
		}
		return encode(struct {
			Case        contract.ReviewCaseV1  `json:"case"`
			Attestation contract.AttestationV1 `json:"attestation"`
		}{caseFact, attestation})
	case "claim":
		var mutation reviewport.ClaimAssignmentMutationV1
		if err := decodeInput(stdin, &mutation); err != nil {
			return err
		}
		caseFact, assignment, err := client.ClaimV1(ctx, mutation)
		if err != nil {
			return err
		}
		return encode(struct {
			Case       contract.ReviewCaseV1         `json:"case"`
			Assignment contract.ReviewerAssignmentV1 `json:"assignment"`
		}{caseFact, assignment})
	case "cancel":
		var command service.CancelCommandV1
		if err := decodeInput(stdin, &command); err != nil {
			return err
		}
		value, err := client.CancelV1(ctx, command)
		if err != nil {
			return err
		}
		return encode(value)
	case "attach-evidence":
		var value contract.EvidenceAttachmentV1
		if err := decodeInput(stdin, &value); err != nil {
			return err
		}
		created, err := client.AttachEvidenceV1(ctx, value)
		if err != nil {
			return err
		}
		return encode(created)
	default:
		return usage()
	}
}

func decodeInput(reader io.Reader, target any) error {
	payload, err := io.ReadAll(io.LimitReader(reader, core.MaxCanonicalDocumentBytes+1))
	if err != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "CLI input could not be read")
	}
	if len(payload) > core.MaxCanonicalDocumentBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "CLI input exceeds its bound")
	}
	return core.DecodeStrictJSON(payload, target)
}
func usage() error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "usage: praxis-review submit|list|show|events|watch|claim|approve|deny|request-changes|attach-evidence|cancel; mutation JSON is read from stdin")
}

type stringListFlag []string

func (v *stringListFlag) String() string { return strings.Join(*v, ",") }
func (v *stringListFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" || len(*v) >= contract.MaxListItemsV1 {
		return fmt.Errorf("invalid repeated value %s", strconv.Quote(value))
	}
	*v = append(*v, value)
	return nil
}
