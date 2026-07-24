# 官方 Prompt 上游原件留存清单

留存时间：2026-07-18（Asia/Shanghai）。本目录保存从官方仓库不可变commit直接取得的原始bytes，供后续Prompt设计与变换审计使用。**不要格式化、改写或把这些文件直接当成Praxis PromptAsset**；所有候选仍须经过Provenance、变换、独立Review和pre-release lifecycle。

文件SHA-256按本地原始bytes计算。原仓库许可证原件一并留存；存在许可证缺口或正文不公开时明确标注，不做推断。

## OpenAI Codex

commit `7c4aaf28c253161f1ed9a4fccc6229b1a4510891`

| 原件 | bytes | SHA-256 |
|---|---:|---|
| [default.md](openai-codex/default.md) | 20903 | `ac8ae107a0d72fe3476b430afb161ea4e67da2e446d778aefc44828160559807` |
| [LICENSE](openai-codex/LICENSE) | 10926 | `d17f227e4df5da1600391338865ce0f3055211760a36688f816941d58232d8dc` |

来源：[`openai/codex`](https://github.com/openai/codex/tree/7c4aaf28c253161f1ed9a4fccc6229b1a4510891)。

## Google Gemini CLI

commit `3ff5ba20fc1ad7d867218bbdb34756eb54d6eccb`

| 原件 | bytes | SHA-256 |
|---|---:|---|
| [promptProvider.ts](gemini-cli/promptProvider.ts) | 11751 | `8dc79837635be5b2288a40a331c20c24024929068aa3f0e74239ebe3a90efa78` |
| [snippets.ts](gemini-cli/snippets.ts) | 68680 | `7411a475e57bf80d01269cb1740ee76d9df0529dc583b4045ccd4a7ce6d4b776` |
| [snippets.legacy.ts](gemini-cli/snippets.legacy.ts) | 56560 | `efed3124be0c59bbf029001dab2e2d04167f13e3ddb4eebd9a8451d7e77be416` |
| [system-prompt.md](gemini-cli/system-prompt.md) | 4634 | `46d42b94733e625d548b3022d99a3a01211ae6f73e7554413e9da45444528241` |
| [LICENSE](gemini-cli/LICENSE) | 11357 | `58d1e17ffe5109a7ae296caafcadfdbe6a7d176f0bc4ab01e12a689b0499d8bd` |

来源：[`google-gemini/gemini-cli`](https://github.com/google-gemini/gemini-cli/tree/3ff5ba20fc1ad7d867218bbdb34756eb54d6eccb)。composer与modern/legacy snippets必须整体审计，不能只取facade。

## Moonshot Kimi Code

commit `7d393b56fb324773fad0af58c7da52b254365cb4`

| 原件 | bytes | SHA-256 |
|---|---:|---|
| [system.md](kimi-code/system.md) | 20726 | `68662e7d9bb565d6c3606b58a9c13dd5377bd47474e10ed8b39ef347a053219c` |
| [promptPrefix.ts](kimi-code/promptPrefix.ts) | 878 | `44afe91ba53db33bc4ea1b158e062b02f3d5a0520bbe3a747cb935a7fb7d4c93` |
| [LICENSE](kimi-code/LICENSE) | 1068 | `23cc68e17992e0b512ae2e80afc5787d7d8e0fbfbdb4fff54ec0245508fa400e` |

来源：[`MoonshotAI/kimi-code`](https://github.com/MoonshotAI/kimi-code/tree/7d393b56fb324773fad0af58c7da52b254365cb4)。模板中的OS/shell/time/workdir/project/skills是动态placeholder，不属于稳定正文。

## xAI Grok

Grok Build commit `98c3b2438aa922fbbe6178a5c0a4c48f85edc8ce`；Grok Prompts commit `a7c186f5ccac95875c0041aed60398f6ecb6d6c7`。

| 原件 | bytes | SHA-256 |
|---|---:|---|
| [prompt.md](grok-build/prompt.md) | 4638 | `c805ee840c5550f501432bf27ae454c7f59f3de4e331ae5107913b4c9f49dafb` |
| [template.rs](grok-build/template.rs) | 34990 | `61074de6dbdd95cff34c9482414359a28f11714060116a8d3783331d4a0f1e3a` |
| [Grok Build LICENSE](grok-build/LICENSE) | 11388 | `116f7778b9802e569b7fa3a532b17bd80eb13c67837def01eed093d4ea472f28` |
| [grok_4_code_rc1_safety_prompt.txt](grok-prompts/grok_4_code_rc1_safety_prompt.txt) | 1083 | `c59f95de2fdd1e6954d1f332b9364f6b9ccf5ebdce082ea524cd6427cba920d6` |
| [Grok Prompts LICENSE](grok-prompts/LICENSE) | 34523 | `0d96a4ff68ad6d4b6f1f30f713b18d5184912ba8dd389f86aa7710db079abcb0` |

来源：[`xai-org/grok-build`](https://github.com/xai-org/grok-build/tree/98c3b2438aa922fbbe6178a5c0a4c48f85edc8ce)、[`xai-org/grok-prompts`](https://github.com/xai-org/grok-prompts/tree/a7c186f5ccac95875c0041aed60398f6ecb6d6c7)。`prompt_encrypted.rs`不是可审计明文，本目录不把它冒充Prompt原文；其远端digest只保留在上游审计文档。

## Anthropic Claude Agent SDK

commit `f604b8c5dd18d2503c8daf52da495ad8db3aa92e`

| 原件 | bytes | SHA-256 |
|---|---:|---|
| [types.py](claude-agent-sdk/types.py) | 75580 | `06c1215aa27806a5221dc44c639afc47f4ef06b872dc3f94384d30d69fa92362` |
| [system_prompt.py](claude-agent-sdk/system_prompt.py) | 2553 | `5e4aadeb8002b409f8955284328267cae2858db0997d98dcab695a0fa055ba45` |
| [LICENSE](claude-agent-sdk/LICENSE) | 1070 | `cebdde8a8fb9ee59e5eaaed19578bf8085aa7047562259c31f38f225c26f6812` |

来源：[`anthropics/claude-agent-sdk-python`](https://github.com/anthropics/claude-agent-sdk-python/tree/f604b8c5dd18d2503c8daf52da495ad8db3aa92e)。这里只留存SDK preset nominal与示例，**不包含Claude Code preset正文**。

## DeepSeek Coder

commit `2f9fd85927c669dae3c0fbb2d607274023af243e`

| 原件 | bytes | SHA-256 |
|---|---:|---|
| [README.md](deepseek-coder/README.md) | 20346 | `aa0a95ca037f1d2c641421417abcaabaca230696cfc79b84cfea235153b8f6f2` |
| [LICENSE-CODE](deepseek-coder/LICENSE-CODE) | 1065 | `6e4c38e1172f42fdbff13edf9a7a017679fb82b0fde415a3e8b3c31c6ed4a4e4` |

来源：[`deepseek-ai/DeepSeek-Coder`](https://github.com/deepseek-ai/DeepSeek-Coder/tree/2f9fd85927c669dae3c0fbb2d607274023af243e)。README中的instruct/chat template是模型模板，不是完整Coding Agent prompt。

## MiniMax

M2.5 commit `0fe00c843c16e7081a9631daeafc11288f5f871c`；CLI commit `f48a4e7703af484d412aacba0d06ccb7b70eaa79`。

| 原件 | bytes | SHA-256 |
|---|---:|---|
| [MiniMax-M2.5 README.md](minimax-m2_5/README.md) | 20087 | `0b64d5b63f22cf1afda49c4fd6c189791c2dd68324a00e7c1df3debd672bdf3c` |
| [MiniMax CLI prompt.ts](minimax-cli/prompt.ts) | 3024 | `b0f26ce3ab5c5aeb542911d8ab8b5adca05e076ac8d7522427d3ce0c5a21f3a9` |

来源：[`MiniMax-AI/MiniMax-M2.5`](https://github.com/MiniMax-AI/MiniMax-M2.5/tree/0fe00c843c16e7081a9631daeafc11288f5f871c)、[`MiniMax-AI/cli`](https://github.com/MiniMax-AI/cli/tree/f48a4e7703af484d412aacba0d06ccb7b70eaa79)。本次commit未发现可闭合的根LICENSE；CLI `prompt.ts`是终端交互helper，不是LLM prompt，留作反例证据。

## T3Code兼容合同

commit `8b5469863ae1dd696e696de30240ec3da607962d`

| 原件 | bytes | SHA-256 |
|---|---:|---|
| [provider.ts](t3code/provider.ts) | 4489 | `45501bda98271090f3c624806697144069d26ee75746d168e32e61d3a42fc0bb` |
| [model.ts](t3code/model.ts) | 8142 | `75e74fa9493f49a8613d03129365789f58e731e7936cd75ca54793cc7052ecdf` |
| [LICENSE](t3code/LICENSE) | 1070 | `935d8f2af0c703f9c39517ee57cc4930b19d02d533be930b63f0e82f93614b43` |

来源：[`pingdotgg/t3code`](https://github.com/pingdotgg/t3code/tree/8b5469863ae1dd696e696de30240ec3da607962d)。这些是消费端兼容合同，不是Prompt来源或Actual Injection事实。

## 完整性规则

- 后续处理只能读取本目录原件，不能原位编辑；变换结果写入新的candidate资产并记录Input/Output digest。
- 更新上游时必须新增commit目录或新版本Manifest，不能覆盖本次原件。
- 文件存在与hash一致不等于许可证已批准、Prompt已Review或模型当前适用。
- Claude preset正文、MiniMax license、Model Invoker exact Profile current等缺口继续Fail Closed。
