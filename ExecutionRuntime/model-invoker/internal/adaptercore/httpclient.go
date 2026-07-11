package adaptercore

import "net/http"

// CloneHTTPClientWithoutRedirects returns an independent shallow clone that
// never follows redirects. The caller's client and CheckRedirect callback are
// left untouched.
func CloneHTTPClientWithoutRedirects(source *http.Client) *http.Client {
	if source == nil {
		source = http.DefaultClient
	}
	cloned := *source
	cloned.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &cloned
}
