package bedrockruntime

import (
	"context"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	bedrockprotocol "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/bedrock"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/smithy-go/auth/bearer"
)

type staticCredentials struct{ value aws.Credentials }

func (s staticCredentials) Retrieve(context.Context) (aws.Credentials, error) { return s.value, nil }

type renewableBearer struct{ source BearerTokenProvider }

func (p renewableBearer) RetrieveBearerToken(ctx context.Context) (bearer.Token, error) {
	value, expires, err := p.source.Token(ctx)
	if err != nil {
		return bearer.Token{}, err
	}
	if value == "" {
		return bearer.Token{}, fmt.Errorf("bedrock runtime: token provider returned an empty token")
	}
	if expires.IsZero() || !expires.After(time.Now()) {
		return bearer.Token{}, fmt.Errorf("bedrock runtime: token provider returned a missing or expired expiry")
	}
	return bearer.Token{Value: value, CanExpire: true, Expires: expires}, nil
}

func newSDKClient(config Config) (bedrockprotocol.Client, error) {
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(config.HTTPClient)
	httpClient = adaptercore.CloneHTTPClientWithResponseCapture(httpClient)
	awsCfg := aws.Config{Region: config.Region, HTTPClient: httpClient, RetryMaxAttempts: 1, BaseEndpoint: optional(config.BaseURL)}
	switch config.CredentialMode {
	case CredentialSigV4:
		awsCfg.Credentials = aws.NewCredentialsCache(staticCredentials{value: aws.Credentials{AccessKeyID: config.AccessKeyID, SecretAccessKey: config.SecretAccessKey, SessionToken: config.SessionToken, Source: "praxis-explicit"}})
	case CredentialBearer:
		awsCfg.AuthSchemePreference = []string{"httpBearerAuth"}
		if config.TokenProvider != nil {
			awsCfg.BearerAuthTokenProvider = renewableBearer{source: config.TokenProvider}
		} else {
			awsCfg.BearerAuthTokenProvider = bearer.StaticTokenProvider{Token: bearer.Token{Value: config.BearerToken}}
		}
	case CredentialDefaultChain:
		options := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(config.Region), awsconfig.WithHTTPClient(httpClient)}
		if config.Profile != "" {
			options = append(options, awsconfig.WithSharedConfigProfile(config.Profile))
		}
		loaded, err := awsconfig.LoadDefaultConfig(context.Background(), options...)
		if err != nil {
			return nil, fmt.Errorf("bedrock runtime: load default AWS configuration: %w", err)
		}
		awsCfg = loaded
		awsCfg.RetryMaxAttempts = 1
		awsCfg.BaseEndpoint = optional(config.BaseURL)
	}
	native := awsruntime.NewFromConfig(awsCfg, func(options *awsruntime.Options) {
		options.RetryMaxAttempts = 1
		options.BaseEndpoint = optional(config.BaseURL)
	})
	return bedrockprotocol.NewSDKClient(native), nil
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
