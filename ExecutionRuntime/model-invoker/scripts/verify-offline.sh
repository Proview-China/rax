#!/usr/bin/env bash
set -euo pipefail

module_root="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$module_root"

# A developer shell may contain real provider or subscription credentials.
# Offline verification must be deterministic and unable to opt into smoke tests.
credential_variables=(
  OPENAI_API_KEY
  ANTHROPIC_API_KEY
  GEMINI_API_KEY
  XAI_API_KEY
  DEEPSEEK_API_KEY
  KIMI_API_KEY
  MOONSHOT_API_KEY
  MINIMAX_API_KEY
  MINIMAX_TOKEN_PLAN_API_KEY
  ZAI_API_KEY
  MIMO_API_KEY
  DASHSCOPE_API_KEY
  AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY
  AWS_SESSION_TOKEN
  AWS_PROFILE
  AWS_DEFAULT_PROFILE
  AWS_REGION
  AWS_DEFAULT_REGION
  AWS_BEARER_TOKEN_BEDROCK
  ANTHROPIC_AWS_API_KEY
  AWS_WEB_IDENTITY_TOKEN_FILE
  AWS_ROLE_ARN
  AWS_CONTAINER_CREDENTIALS_FULL_URI
  AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
  GOOGLE_APPLICATION_CREDENTIALS
  GOOGLE_API_KEY
  GOOGLE_CLOUD_PROJECT
  GOOGLE_CLOUD_LOCATION
  AZURE_CLIENT_ID
  AZURE_TENANT_ID
  AZURE_CLIENT_SECRET
  AZURE_CLIENT_CERTIFICATE_PATH
  AZURE_OPENAI_API_KEY
  PRAXIS_OPENAI_SMOKE
  PRAXIS_ANTHROPIC_SMOKE
  PRAXIS_GEMINI_SMOKE
  PRAXIS_BEDROCK_MANTLE_SMOKE
  PRAXIS_BEDROCK_RUNTIME_SMOKE
  PRAXIS_VERTEX_SMOKE
  PRAXIS_AZURE_OPENAI_SMOKE
  PRAXIS_LIVE_TESTS
  PRAXIS_DEEPSEEK_LIVE_TESTS
  DEEPSEEK_SMOKE_MODEL
  PRAXIS_KIMI_LIVE_TESTS
  KIMI_SMOKE_MODEL
  PRAXIS_MINIMAX_LIVE_TESTS
  MINIMAX_SMOKE_MODEL
  PRAXIS_ZAI_LIVE_TESTS
  ZAI_SMOKE_MODEL
  PRAXIS_MIMO_LIVE_TESTS
  MIMO_SMOKE_MODEL
  PRAXIS_QWEN_LIVE_TESTS
  QWEN_SMOKE_WORKSPACE_ID
  QWEN_SMOKE_REGION
  QWEN_SMOKE_MODEL
  PRAXIS_XAI_LIVE_TESTS
  XAI_SMOKE_MODEL
  OPENAI_SMOKE_MODEL
  ANTHROPIC_SMOKE_MODEL
  GEMINI_SMOKE_MODEL
  BEDROCK_MANTLE_SMOKE_MODEL
  BEDROCK_RUNTIME_SMOKE_MODEL
  BEDROCK_SMOKE_PROJECT_REF
  VERTEX_SMOKE_MODEL
  VERTEX_SMOKE_DEPLOYMENT_REF
  AZURE_OPENAI_ENDPOINT
  AZURE_OPENAI_REGION
  AZURE_OPENAI_DEPLOYMENT
)
for variable in "${credential_variables[@]}"; do
  unset "$variable"
done
unset GOFLAGS

# Dependency acquisition is the only step allowed to use the configured module
# proxy. All verification commands below run with outbound HTTP proxies pointed
# at a closed loopback port; httptest loopback servers remain reachable.
go mod download
go mod verify

export HTTP_PROXY="http://127.0.0.1:1"
export HTTPS_PROXY="http://127.0.0.1:1"
export ALL_PROXY="http://127.0.0.1:1"
export NO_PROXY="127.0.0.1,localhost,::1"
export http_proxy="$HTTP_PROXY"
export https_proxy="$HTTPS_PROXY"
export all_proxy="$ALL_PROXY"
export no_proxy="$NO_PROXY"

unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  echo "gofmt is required for:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go mod tidy -diff
git diff --check
go vet ./...
go test -count=1 ./...
go test -shuffle=on -count=1 ./...
go test -race -count=1 ./...

# Compile the explicitly gated smoke-test package without selecting any test.
# With the credentials and confirmation flags removed above, this cannot make
# a real provider or subscription request.
go test -tags=integration -run '^$' ./tests/integration

# Keep the cross-language schema and checked-in Markdown block visible as an
# explicit CI surface even though they are also included by ./....
go test -count=1 ./tests/catalogassets

echo "model-invoker offline verification passed"
