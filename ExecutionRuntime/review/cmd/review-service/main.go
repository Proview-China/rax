package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	reviewhttp "github.com/Proview-China/rax/ExecutionRuntime/review/api/http"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	storesqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type authEntryV1 struct {
	Token        string        `json:"token"`
	TenantID     core.TenantID `json:"tenant_id"`
	SubjectID    string        `json:"subject_id"`
	Capabilities []string      `json:"capabilities"`
}
type authConfigV1 struct {
	Entries    []authEntryV1 `json:"entries"`
	TTLSeconds int64         `json:"ttl_seconds"`
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	now := time.Now()
	database := os.Getenv("PRAXIS_REVIEW_DB")
	address := os.Getenv("PRAXIS_REVIEW_ADDR")
	if address == "" {
		address = "127.0.0.1:8087"
	}
	if database == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "PRAXIS_REVIEW_DB is required")
	}
	var authConfig authConfigV1
	if err := core.DecodeStrictJSON([]byte(os.Getenv("PRAXIS_REVIEW_AUTH_JSON")), &authConfig); err != nil {
		return err
	}
	if len(authConfig.Entries) == 0 || len(authConfig.Entries) > 1024 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review service auth entries are empty or too large")
	}
	if authConfig.TTLSeconds <= 0 || authConfig.TTLSeconds > 30*24*60*60 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "review service auth TTL is invalid")
	}
	tokens := make(map[string]reviewhttp.PrincipalV1, len(authConfig.Entries))
	for _, entry := range authConfig.Entries {
		if _, exists := tokens[entry.Token]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "review service auth token is duplicated")
		}
		capabilities := append([]string(nil), entry.Capabilities...)
		sort.Strings(capabilities)
		tokens[entry.Token] = reviewhttp.PrincipalV1{TenantID: entry.TenantID, SubjectID: entry.SubjectID, Capabilities: capabilities, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Duration(authConfig.TTLSeconds) * time.Second).UnixNano()}
	}
	auth, err := reviewhttp.NewStaticBearerAuthenticatorV1(tokens)
	if err != nil {
		return err
	}
	cursorKey, err := hex.DecodeString(os.Getenv("PRAXIS_REVIEW_CURSOR_KEY_HEX"))
	if err != nil || len(cursorKey) < 32 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "PRAXIS_REVIEW_CURSOR_KEY_HEX must contain at least 32 bytes")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	store, err := storesqlite.Open(ctx, storesqlite.Config{Path: database, Clock: time.Now})
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.IntegrityCheckV1(ctx); err != nil {
		return err
	}
	owner, err := service.New(store, time.Now)
	if err != nil {
		return err
	}
	handler, err := reviewhttp.New(reviewhttp.Config{Service: owner, Authenticator: auth, Clock: time.Now, CursorKey: cursorKey})
	if err != nil {
		return err
	}
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 0, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 64 << 10}
	cert, key := os.Getenv("PRAXIS_REVIEW_TLS_CERT"), os.Getenv("PRAXIS_REVIEW_TLS_KEY")
	if (cert == "") != (key == "") {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "review TLS cert and key must be configured together")
	}
	if cert == "" && !loopbackAddress(address) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "review service requires TLS outside loopback")
	}
	errors := make(chan error, 1)
	go func() {
		if cert != "" {
			errors <- server.ListenAndServeTLS(cert, key)
		} else {
			errors <- server.ListenAndServe()
		}
	}()
	select {
	case err := <-errors:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		return server.Shutdown(shutdown)
	}
}
func loopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
