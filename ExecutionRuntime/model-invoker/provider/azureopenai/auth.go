package azureopenai

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type credentialSource interface {
	header(context.Context) (string, string, error)
}
type staticAPIKey string

func (k staticAPIKey) header(context.Context) (string, string, error) {
	return "api-key", string(k), nil
}

type renewableAPIKey struct{ source APIKeyProvider }

func (p renewableAPIKey) header(ctx context.Context) (string, string, error) {
	v, e, err := p.source.APIKey(ctx)
	if err != nil {
		return "", "", err
	}
	if v == "" || e.IsZero() || !e.After(time.Now()) {
		return "", "", fmt.Errorf("azure openai: API key provider returned an empty or expired key")
	}
	return "api-key", v, nil
}

type entraToken struct{ source AccessTokenProvider }

func (p entraToken) header(ctx context.Context) (string, string, error) {
	v, e, err := p.source.AccessToken(ctx)
	if err != nil {
		return "", "", err
	}
	if v == "" || e.IsZero() || !e.After(time.Now()) {
		return "", "", fmt.Errorf("azure openai: Entra ID provider returned an empty or expired token")
	}
	return "authorization", "Bearer " + v, nil
}

type authTransport struct {
	next       http.RoundTripper
	source     credentialSource
	apiVersion string
}

func (t authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	clone.Header.Del("authorization")
	clone.Header.Del("api-key")
	name, value, err := t.source.header(clone.Context())
	if err != nil {
		return nil, err
	}
	clone.Header.Set(name, value)
	if t.apiVersion != "" {
		u := *clone.URL
		q := url.Values{}
		q.Set("api-version", t.apiVersion)
		u.RawQuery = q.Encode()
		clone.URL = &u
	} else if clone.URL.RawQuery != "" {
		return nil, fmt.Errorf("azure openai: v1 request must not contain api-version or any query")
	}
	return t.next.RoundTrip(clone)
}
