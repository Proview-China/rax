# Factory A/B双层信任矩阵 v1candidate

本文件由 `cmd/factorytrustgen` 从 live Catalog、Builtin Registry 与 `internal/trustmatrix` 合同生成；CSV保持严格18个Factory数据行，本文展开protocol/profile子合同。

| Factory | FactoryVersion | Scope | callable | host-blocked | protocol/profile子合同 |
|---|---|---:|---:|---:|---:|
| `builtin/alibaba-plan` | `v1candidate` | `host_blocked_subscription` | 0 | 6 | 6 |
| `builtin/anthropic` | `v1candidate` | `default_active` | 1 | 0 | 1 |
| `builtin/aws-bedrock-mantle` | `v1candidate` | `default_active` | 6 | 0 | 6 |
| `builtin/aws-bedrock-runtime` | `v1candidate` | `default_active` | 4 | 0 | 4 |
| `builtin/azure-openai` | `v1candidate` | `default_active` | 6 | 0 | 4 |
| `builtin/deepseek` | `v1candidate` | `default_active` | 2 | 0 | 2 |
| `builtin/gemini` | `v1candidate` | `default_active` | 1 | 0 | 1 |
| `builtin/google-vertex-ai` | `v1candidate` | `default_active` | 5 | 0 | 5 |
| `builtin/kimi` | `v1candidate` | `default_active` | 1 | 0 | 1 |
| `builtin/kimi-code` | `v1candidate` | `host_blocked_subscription` | 0 | 2 | 2 |
| `builtin/mimo-token-plan` | `v1candidate` | `host_blocked_subscription` | 0 | 6 | 6 |
| `builtin/minimax` | `v1candidate` | `default_active` | 3 | 0 | 3 |
| `builtin/minimax-token-plan` | `v1candidate` | `host_blocked_subscription` | 0 | 2 | 2 |
| `builtin/openai` | `v1candidate` | `default_active` | 2 | 0 | 2 |
| `builtin/qwen` | `v1candidate` | `default_active` | 4 | 0 | 4 |
| `builtin/xai` | `v1candidate` | `default_active` | 1 | 0 | 1 |
| `builtin/xiaomi-mimo` | `v1candidate` | `default_active` | 2 | 0 | 2 |
| `builtin/zai` | `v1candidate` | `default_active` | 1 | 0 | 1 |

## `builtin/alibaba-plan`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `alibaba.coding-plan.cn` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `chat_completions` | `alibaba.coding-plan.intl` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `chat_completions` | `alibaba.token-plan-team.cn-beijing` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `alibaba.coding-plan.cn` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `alibaba.coding-plan.intl` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `alibaba.token-plan-team.cn-beijing` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/anthropic`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `messages` | `anthropic.default` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/aws-bedrock-mantle`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `aws.bedrock-mantle.api-key` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `chat_completions` | `aws.bedrock-mantle.sigv4` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `aws.bedrock-mantle.api-key` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `aws.bedrock-mantle.sigv4` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `responses` | `aws.bedrock-mantle.api-key` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `responses` | `aws.bedrock-mantle.sigv4` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/aws-bedrock-runtime`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `bedrock_converse` | `aws.bedrock-runtime.bearer` | 1 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |
| `bedrock_converse` | `aws.bedrock-runtime.sigv4` | 1 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |
| `bedrock_invoke_model` | `aws.bedrock-runtime.bearer` | 1 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |
| `bedrock_invoke_model` | `aws.bedrock-runtime.sigv4` | 1 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |

- `bedrock_converse/aws.bedrock-runtime.bearer` indirect理由：Bedrock native APIs bind the selected model in the request URI/input and do not return an authoritative actual-model field; portable Model is the request projection.
- `bedrock_converse/aws.bedrock-runtime.sigv4` indirect理由：Bedrock native APIs bind the selected model in the request URI/input and do not return an authoritative actual-model field; portable Model is the request projection.
- `bedrock_invoke_model/aws.bedrock-runtime.bearer` indirect理由：Bedrock native APIs bind the selected model in the request URI/input and do not return an authoritative actual-model field; portable Model is the request projection.
- `bedrock_invoke_model/aws.bedrock-runtime.sigv4` indirect理由：Bedrock native APIs bind the selected model in the request URI/input and do not return an authoritative actual-model field; portable Model is the request projection.

## `builtin/azure-openai`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `azure.openai.api-key` | 2 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |
| `chat_completions` | `azure.openai.entra` | 2 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |
| `responses` | `azure.openai.api-key` | 1 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |
| `responses` | `azure.openai.entra` | 1 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |

- `chat_completions/azure.openai.api-key` indirect理由：Azure request identity is the configured deployment; compatibility responses may echo a different native model, so portable Model is an explicit deployment projection.
- `chat_completions/azure.openai.entra` indirect理由：Azure request identity is the configured deployment; compatibility responses may echo a different native model, so portable Model is an explicit deployment projection.
- `responses/azure.openai.api-key` indirect理由：Azure request identity is the configured deployment; compatibility responses may echo a different native model, so portable Model is an explicit deployment projection.
- `responses/azure.openai.entra` indirect理由：Azure request identity is the configured deployment; compatibility responses may echo a different native model, so portable Model is an explicit deployment projection.

## `builtin/deepseek`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `deepseek.default.openai` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `deepseek.default.anthropic` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/gemini`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `generate_content` | `google.gemini-developer.default` | 1 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |

- `generate_content/google.gemini-developer.default` indirect理由：GenerateContent portable Model is the exact request projection; upstream modelVersion is metadata and is not treated as authoritative actual-model proof.

## `builtin/google-vertex-ai`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `google.vertex-ai.adc` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `chat_completions` | `google.vertex-ai.api-key` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `generate_content` | `google.vertex-ai.adc` | 1 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |
| `generate_content` | `google.vertex-ai.api-key` | 1 | `pass/exact_endpoint_policy` | `not_applicable/indirect` | `pass/adapter_owned_receipt` | `not_applicable/indirect` |
| `messages` | `google.vertex-ai.adc` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

- `generate_content/google.vertex-ai.adc` indirect理由：GenerateContent portable Model is the exact request projection; upstream modelVersion is metadata and is not treated as authoritative actual-model proof.
- `generate_content/google.vertex-ai.api-key` indirect理由：GenerateContent portable Model is the exact request projection; upstream modelVersion is metadata and is not treated as authoritative actual-model proof.

## `builtin/kimi`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `kimi.platform.cn` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/kimi-code`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `kimi.code-membership.global` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `kimi.code-membership.global.messages` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/mimo-token-plan`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `mimo.token-plan.ams` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `chat_completions` | `mimo.token-plan.cn` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `chat_completions` | `mimo.token-plan.sgp` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `mimo.token-plan.ams` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `mimo.token-plan.cn` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `mimo.token-plan.sgp` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/minimax`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `minimax.platform.global.chat_completions` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `minimax.platform.global.messages` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `responses` | `minimax.platform.global.responses` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/minimax-token-plan`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `minimax.token-plan.global` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `minimax.token-plan.global.messages` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/openai`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `openai.default` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `responses` | `openai.default` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/qwen`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `alibaba.model-studio.ap-southeast-1.chat_completions` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `chat_completions` | `alibaba.model-studio.cn-beijing.chat_completions` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `responses` | `alibaba.model-studio.ap-southeast-1.responses` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `responses` | `alibaba.model-studio.cn-beijing.responses` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/xai`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `responses` | `xai.api.global.responses` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/xiaomi-mimo`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `xiaomi.mimo.global.chat_completions` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
| `messages` | `xiaomi.mimo.global.messages` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |

## `builtin/zai`

| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |
|---|---|---:|---|---|---|---|
| `chat_completions` | `zai.platform.global` | 1 | `pass/exact_endpoint_policy` | `pass/exact` | `pass/adapter_owned_receipt` | `pass/exact` |
