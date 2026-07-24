package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/engineeringapi"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/offlineapi"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

const cliUsageV1 = "usage: context recipe validate|compare|compile|preview OR context frame inspect OR context cache inspect OR context prompt validate|preview OR context evaluation prepare|admit OR context feedback build"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if operation, ok := commandOperationV1(args); ok {
		return runOfflineV1(ctx, operation, stdin, stdout, stderr)
	}
	if operation, ok := engineeringCommandOperationV1(args); ok {
		return runEngineeringV1(ctx, operation, stdin, stdout, stderr)
	}
	writeCLIErrorV1(stderr, sdk.OfflineErrorUnsupportedV1, "", "command", cliUsageV1)
	return 2
}

func runOfflineV1(ctx context.Context, operation sdk.OfflineSDKOperationV1, stdin io.Reader, stdout, stderr io.Writer) int {
	requestCap, _, _ := sdk.WireCapsV1(operation)
	payload, err := readRequestV1(ctx, stdin, requestCap)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			writeCLIErrorV1(stderr, sdk.OfflineErrorCanceledV1, operation, "stdin", "operation canceled")
			return 5
		}
		if errors.Is(err, context.DeadlineExceeded) {
			writeCLIErrorV1(stderr, sdk.OfflineErrorDeadlineExceededV1, operation, "stdin", "deadline exceeded")
			return 5
		}
		writeCLIErrorV1(stderr, sdk.OfflineErrorInternalFailureV1, operation, "stdin", "failed to read request")
		return 1
	}
	if uint64(len(payload)) > requestCap {
		writeCLIErrorV1(stderr, sdk.OfflineErrorLimitExceededV1, operation, "stdin", "request exceeds operation hard wire cap")
		return 2
	}
	response, err := (offlineapi.ServiceV1{}).ExecuteJSON(ctx, operation, payload)
	if err != nil {
		return writeServiceErrorV1(stderr, operation, err)
	}
	if _, err := stdout.Write(response); err != nil {
		writeCLIErrorV1(stderr, sdk.OfflineErrorInternalFailureV1, operation, "stdout", "failed to write response")
		return 1
	}
	if _, err := stdout.Write([]byte("\n")); err != nil {
		return 1
	}
	return 0
}

func runEngineeringV1(ctx context.Context, operation sdk.ContextEngineeringOperationV1, stdin io.Reader, stdout, stderr io.Writer) int {
	requestCap := sdk.DefaultContextEngineeringLimitsV1().MaxWireBytes
	payload, err := readRequestV1(ctx, stdin, requestCap)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			writeEngineeringCLIErrorV1(stderr, sdk.EngineeringErrorCanceledV1, operation, "stdin", "operation canceled")
			return 5
		}
		if errors.Is(err, context.DeadlineExceeded) {
			writeEngineeringCLIErrorV1(stderr, sdk.EngineeringErrorDeadlineExceededV1, operation, "stdin", "deadline exceeded")
			return 5
		}
		writeEngineeringCLIErrorV1(stderr, sdk.EngineeringErrorInternalFailureV1, operation, "stdin", "failed to read request")
		return 1
	}
	if uint64(len(payload)) > requestCap {
		writeEngineeringCLIErrorV1(stderr, sdk.EngineeringErrorLimitExceededV1, operation, "stdin", "request exceeds engineering hard wire cap")
		return 2
	}
	response, err := (engineeringapi.ServiceV1{}).ExecuteJSON(ctx, operation, payload)
	if err != nil {
		return writeEngineeringServiceErrorV1(stderr, operation, err)
	}
	if _, err := stdout.Write(response); err != nil {
		writeEngineeringCLIErrorV1(stderr, sdk.EngineeringErrorInternalFailureV1, operation, "stdout", "failed to write response")
		return 1
	}
	if _, err := stdout.Write([]byte("\n")); err != nil {
		return 1
	}
	return 0
}

func readRequestV1(ctx context.Context, reader io.Reader, max uint64) ([]byte, error) {
	if ctx == nil {
		return nil, context.Canceled
	}
	limited := io.LimitReader(reader, int64(max)+1)
	var buffer bytes.Buffer
	chunk := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, err := limited.Read(chunk)
		if count > 0 {
			if uint64(buffer.Len()) > max-uint64(count) {
				buffer.Write(chunk[:count])
				return buffer.Bytes(), nil
			}
			buffer.Write(chunk[:count])
		}
		if err == io.EOF {
			return buffer.Bytes(), ctx.Err()
		}
		if err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, io.ErrNoProgress
		}
	}
}

func commandOperationV1(args []string) (sdk.OfflineSDKOperationV1, bool) {
	if len(args) != 2 {
		return "", false
	}
	switch args[0] + " " + args[1] {
	case "recipe validate":
		return sdk.OfflineValidateRecipeV1, true
	case "recipe compare":
		return sdk.OfflineCompareRecipesV1, true
	case "recipe compile":
		return sdk.OfflineCompileFrameV1, true
	case "recipe preview":
		return sdk.OfflinePreviewFrameV1, true
	case "frame inspect":
		return sdk.OfflineInspectFrameExactV1, true
	case "cache inspect":
		return sdk.OfflineInspectCachePlanV1, true
	default:
		return "", false
	}
}

func engineeringCommandOperationV1(args []string) (sdk.ContextEngineeringOperationV1, bool) {
	if len(args) != 2 {
		return "", false
	}
	switch args[0] + " " + args[1] {
	case "prompt validate":
		return sdk.EngineeringValidatePromptAssetV1, true
	case "prompt preview":
		return sdk.EngineeringPreviewPromptV1, true
	case "evaluation prepare":
		return sdk.EngineeringPrepareEvaluationV1, true
	case "evaluation admit":
		return sdk.EngineeringAdmitEvaluationV1, true
	case "feedback build":
		return sdk.EngineeringBuildFeedbackV1, true
	default:
		return "", false
	}
}

func writeServiceErrorV1(stderr io.Writer, operation sdk.OfflineSDKOperationV1, err error) int {
	var typed *sdk.OfflineSDKErrorV1
	if !errors.As(err, &typed) {
		writeCLIErrorV1(stderr, sdk.OfflineErrorInternalFailureV1, operation, "operation", "internal failure")
		return 1
	}
	writeCLIErrorV1(stderr, typed.Code, typed.Operation, typed.FieldPath, typed.Message)
	switch typed.Code {
	case sdk.OfflineErrorInvalidArgumentV1, sdk.OfflineErrorLimitExceededV1, sdk.OfflineErrorUnsupportedV1:
		return 2
	case sdk.OfflineErrorNotFoundV1, sdk.OfflineErrorExpiredV1, sdk.OfflineErrorConflictV1:
		return 3
	case sdk.OfflineErrorUnauthorizedV1:
		return 4
	case sdk.OfflineErrorCanceledV1, sdk.OfflineErrorDeadlineExceededV1:
		return 5
	default:
		return 1
	}
}

func writeCLIErrorV1(stderr io.Writer, code sdk.OfflineSDKErrorCodeV1, operation sdk.OfflineSDKOperationV1, path, message string) {
	_ = json.NewEncoder(stderr).Encode(struct {
		Code      sdk.OfflineSDKErrorCodeV1 `json:"code"`
		Operation sdk.OfflineSDKOperationV1 `json:"operation,omitempty"`
		FieldPath string                    `json:"field_path"`
		Message   string                    `json:"message"`
	}{Code: code, Operation: operation, FieldPath: path, Message: message})
}

func writeEngineeringServiceErrorV1(stderr io.Writer, operation sdk.ContextEngineeringOperationV1, err error) int {
	var typed *sdk.ContextEngineeringErrorV1
	if !errors.As(err, &typed) {
		writeEngineeringCLIErrorV1(stderr, sdk.EngineeringErrorInternalFailureV1, operation, "operation", "internal failure")
		return 1
	}
	writeEngineeringCLIErrorV1(stderr, typed.Code, typed.Operation, typed.FieldPath, typed.Message)
	switch typed.Code {
	case sdk.EngineeringErrorInvalidArgumentV1, sdk.EngineeringErrorLimitExceededV1, sdk.EngineeringErrorUnsupportedV1:
		return 2
	case sdk.EngineeringErrorNotFoundV1, sdk.EngineeringErrorExpiredV1, sdk.EngineeringErrorConflictV1:
		return 3
	case sdk.EngineeringErrorUnauthorizedV1:
		return 4
	case sdk.EngineeringErrorCanceledV1, sdk.EngineeringErrorDeadlineExceededV1:
		return 5
	default:
		return 1
	}
}

func writeEngineeringCLIErrorV1(stderr io.Writer, code sdk.ContextEngineeringErrorCodeV1, operation sdk.ContextEngineeringOperationV1, path, message string) {
	_ = json.NewEncoder(stderr).Encode(struct {
		Code      sdk.ContextEngineeringErrorCodeV1 `json:"code"`
		Operation sdk.ContextEngineeringOperationV1 `json:"operation,omitempty"`
		FieldPath string                            `json:"field_path"`
		Message   string                            `json:"message"`
	}{Code: code, Operation: operation, FieldPath: path, Message: message})
}
