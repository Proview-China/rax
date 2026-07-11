package vertex

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

type credentialSource interface {
	authorization(context.Context) (string, string, error)
}
type staticAPIKey string

func (k staticAPIKey) authorization(context.Context) (string, string, error) {
	return "x-goog-api-key", string(k), nil
}

type renewableAPIKey struct{ source APIKeyProvider }

func (p renewableAPIKey) authorization(ctx context.Context) (string, string, error) {
	v, e, err := p.source.APIKey(ctx)
	if err != nil {
		return "", "", err
	}
	if v == "" || e.IsZero() || !e.After(time.Now()) {
		return "", "", fmt.Errorf("vertex: API key provider returned an empty or expired key")
	}
	return "x-goog-api-key", v, nil
}

type renewableAccessToken struct{ source AccessTokenProvider }

func (p renewableAccessToken) authorization(ctx context.Context) (string, string, error) {
	v, e, err := p.source.AccessToken(ctx)
	if err != nil {
		return "", "", err
	}
	if v == "" || e.IsZero() || !e.After(time.Now()) {
		return "", "", fmt.Errorf("vertex: token provider returned an empty or expired token")
	}
	return "authorization", "Bearer " + v, nil
}

type authTransport struct {
	next   http.RoundTripper
	source credentialSource
}

func (t authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	clone.Header.Del("authorization")
	clone.Header.Del("x-goog-api-key")
	if t.source != nil {
		name, value, err := t.source.authorization(clone.Context())
		if err != nil {
			return nil, err
		}
		clone.Header.Set(name, value)
	}
	return t.next.RoundTrip(clone)
}

type oauthTokenSource struct{ source AccessTokenProvider }

func (s oauthTokenSource) Token() (*oauth2.Token, error) {
	value, expires, err := s.source.AccessToken(context.Background())
	if err != nil {
		return nil, err
	}
	if value == "" || expires.IsZero() || !expires.After(time.Now()) {
		return nil, fmt.Errorf("vertex: token provider returned an empty or expired token")
	}
	return &oauth2.Token{AccessToken: value, TokenType: "Bearer", Expiry: expires}, nil
}
