package bedrockmantle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsv4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

type keySource interface {
	key(context.Context) (string, error)
}
type staticKey string

func (s staticKey) key(context.Context) (string, error) { return string(s), nil }

type renewableKey struct{ source APIKeyProvider }

func (p renewableKey) key(ctx context.Context) (string, error) {
	value, expires, err := p.source.APIKey(ctx)
	if err != nil {
		return "", err
	}
	if value == "" || expires.IsZero() || !expires.After(time.Now()) {
		return "", fmt.Errorf("bedrock mantle: key provider returned an empty or expired key")
	}
	return value, nil
}

type staticCredentials struct{ value aws.Credentials }

func (s staticCredentials) Retrieve(context.Context) (aws.Credentials, error) { return s.value, nil }

type authTransport struct {
	next        http.RoundTripper
	region      string
	apiKey      keySource
	credentials aws.CredentialsProvider
	signer      *awsv4.Signer
}

func (t authTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Del("authorization")
	clone.Header.Del("x-api-key")
	if t.apiKey != nil {
		key, err := t.apiKey.key(clone.Context())
		if err != nil {
			return nil, err
		}
		clone.Header.Set("x-api-key", key)
		return t.next.RoundTrip(clone)
	}
	body, err := readAndRestoreBody(clone)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(body)
	credentials, err := t.credentials.Retrieve(clone.Context())
	if err != nil {
		return nil, err
	}
	if err := t.signer.SignHTTP(clone.Context(), credentials, clone, hex.EncodeToString(hash[:]), "bedrock-mantle", t.region, time.Now()); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(clone)
}

func readAndRestoreBody(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	_ = request.Body.Close()
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	return body, nil
}

func credentialSources(config Config) (keySource, aws.CredentialsProvider, error) {
	switch config.CredentialMode {
	case CredentialAPIKey:
		if config.APIKeyProvider != nil {
			return renewableKey{source: config.APIKeyProvider}, nil, nil
		}
		return staticKey(config.APIKey), nil, nil
	case CredentialSigV4:
		return nil, aws.NewCredentialsCache(staticCredentials{value: aws.Credentials{AccessKeyID: config.AccessKeyID, SecretAccessKey: config.SecretAccessKey, SessionToken: config.SessionToken, Source: "praxis-explicit"}}), nil
	case CredentialDefaultChain:
		options := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(config.Region)}
		if config.Profile != "" {
			options = append(options, awsconfig.WithSharedConfigProfile(config.Profile))
		}
		loaded, err := awsconfig.LoadDefaultConfig(context.Background(), options...)
		if err != nil {
			return nil, nil, err
		}
		return nil, loaded.Credentials, nil
	default:
		return nil, nil, fmt.Errorf("unsupported credential mode")
	}
}
