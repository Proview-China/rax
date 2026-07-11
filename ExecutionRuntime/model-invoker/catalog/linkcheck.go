package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPDoer is deliberately injectable. Ordinary catalog validation and tests
// never create a network client or contact an official source implicitly.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type SourceLinkResult struct {
	SourceID   string
	URL        string
	Method     string
	StatusCode int
	Reachable  bool
	Problem    string
}

type SourceLinkChecker struct {
	doer HTTPDoer
}

func NewSourceLinkChecker(doer HTTPDoer) (*SourceLinkChecker, error) {
	if doer == nil {
		return nil, fmt.Errorf("catalog source link checker requires an HTTP doer")
	}
	return &SourceLinkChecker{doer: doer}, nil
}

// Check verifies source reachability in stable input order. Callers must opt in
// with an explicit HTTPDoer; this method is not used by the offline CI entry.
func (c *SourceLinkChecker) Check(ctx context.Context, sources []OfficialSource) ([]SourceLinkResult, error) {
	if c == nil || c.doer == nil {
		return nil, fmt.Errorf("catalog source link checker is not initialized")
	}
	if ctx == nil {
		return nil, fmt.Errorf("catalog source link checker context is nil")
	}
	results := make([]SourceLinkResult, 0, len(sources))
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		result := c.checkOne(ctx, source)
		results = append(results, result)
	}
	return results, nil
}

func (c *SourceLinkChecker) checkOne(ctx context.Context, source OfficialSource) SourceLinkResult {
	result := SourceLinkResult{SourceID: source.ID, URL: source.URL, Method: http.MethodHead}
	response, err := c.do(ctx, http.MethodHead, source.URL)
	if err != nil {
		result.Problem = err.Error()
		return result
	}
	status := response.StatusCode
	closeResponseBody(response)
	if status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		result.Method = http.MethodGet
		response, err = c.do(ctx, http.MethodGet, source.URL)
		if err != nil {
			result.Problem = err.Error()
			return result
		}
		status = response.StatusCode
		closeResponseBody(response)
	}
	result.StatusCode = status
	result.Reachable = status >= http.StatusOK && status < http.StatusBadRequest
	if !result.Reachable {
		result.Problem = fmt.Sprintf("official source returned HTTP %d", status)
	}
	return result
}

func (c *SourceLinkChecker) do(ctx context.Context, method, rawURL string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("construct %s source request: %w", strings.ToLower(method), err)
	}
	request.Header.Set("User-Agent", "Praxis-Catalog-LinkCheck/1")
	if method == http.MethodGet {
		request.Header.Set("Range", "bytes=0-0")
	}
	response, err := c.doer.Do(request)
	if err != nil {
		return nil, fmt.Errorf("check official source: %w", err)
	}
	if response == nil {
		return nil, fmt.Errorf("check official source: HTTP doer returned a nil response")
	}
	return response, nil
}

func closeResponseBody(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	_ = response.Body.Close()
}
