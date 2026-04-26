# oc-go-cc

A Go CLI proxy that lets you use your [OpenCode Go](https://opencode.ai/docs/go/) subscription with [Claude Code](https://docs.anthropic.com/en/docs/claude-code).

`oc-go-cc` sits between Claude Code and OpenCode Go, intercepting Anthropic API requests, transforming them to OpenAI format, and forwarding them to OpenCode Go's endpoint. Claude Code thinks it's talking to Anthropic — but your requests go to affordable open models instead.

## Why?

OpenCode Go gives you access to powerful open coding models for **$5/month** (then $10/month). This proxy makes those models work seamlessly with Claude Code's interface — no patches, no forks, just set two environment variables and go.

## Features

- **Transparent Proxy** — Claude Code sends Anthropic-format requests, proxy transforms to OpenAI format and back
- **Model Routing** — Automatically routes to different models based on context (default, thinking, long context, background)
- **Fallback Chains** — If a model fails, automatically tries the next one in your configured chain
- **Circuit Breaker** — Tracks model health and skips failing models to avoid latency spikes
- **Real-time Streaming** — Full SSE streaming with live OpenAI → Anthropic format transformation
- **Tool Calling** — Proper Anthropic tool_use/tool_result ↔ OpenAI function calling translation
- **Token Counting** — Uses tiktoken (cl100k_base) for accurate token counting and context threshold detection
- **JSON Configuration** — Flexible config file with environment variable overrides and `${VAR}` interpolation
- **Background Mode** — Run as daemon detached from terminal
- **Auto-start on Login** — Launch on system startup via launchd (macOS)

## Installation

### Homebrew (macOS & Linux)

```bash
brew tap samueltuyizere/tap
brew install oc-go-cc
```

### Build from Source

```bash
git clone https://github.com/samueltuyizere/oc-go-cc.git
cd oc-go-cc
make build

# Binary is at bin/oc-go-cc
# Optionally install to $GOPATH/bin
make install
```

### Download a Release Binary

Download the latest release for your platform from the [Releases page](https://github.com/samueltuyizere/oc-go-cc/releases):

| Platform              | File                         |
| --------------------- | ---------------------------- |
| macOS (Apple Silicon) | `oc-go-cc_darwin-arm64`      |
| macOS (Intel)         | `oc-go-cc_darwin-amd64`      |
| Linux (x86_64)        | `oc-go-cc_linux-amd64`       |
| Linux (ARM64)         | `oc-go-cc_linux-arm64`       |
| Windows (x86_64)      | `oc-go-cc_windows-amd64.exe` |
| Windows (ARM64)       | `oc-go-cc_windows-arm64.exe` |

```bash
# Example: macOS Apple Silicon
curl -L -o oc-go-cc https://github.com/samueltuyizere/oc-go-cc/releases/latest/download/oc-go-cc_darwin-arm64
chmod +x oc-go-cc
sudo mv oc-go-cc /usr/local/bin/
```

### Requirements

- An [OpenCode Go](https://opencode.ai/auth) subscription and API key
- Go 1.21+ (only needed if building from source)

## Quick Start

### 1. Initialize Configuration

```bash
oc-go-cc init
```

Creates a default config at `~/.config/oc-go-cc/config.json`.

### 2. Set Your API Key

```bash
export OC_GO_CC_API_KEY=sk-opencode-your-key-here
```

### 3. Start the Proxy

```bash
oc-go-cc serve
```

You'll see output like:

```
Starting oc-go-cc v0.1.0
Listening on 127.0.0.1:3456
Forwarding to: https://opencode.ai/zen/go/v1/chat/completions

Configure Claude Code with:
  export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
  export ANTHROPIC_AUTH_TOKEN=unused
```

#### Running in Background

To run the proxy in the background (detached from terminal):

```bash
oc-go-cc serve --background
# or
oc-go-cc serve -b
```

This starts the server as a background daemon and returns immediately. Logs are written to `~/.config/oc-go-cc/oc-go-cc.log`.

#### Auto-start on Login

To start the proxy automatically when you log in:

```bash
oc-go-cc autostart enable
```

This creates a launchd plist on macOS. To disable:

```bash
oc-go-cc autostart disable
```

Check status:

```bash
oc-go-cc autostart status
```

### 4. Configure Claude Code

In a separate terminal (or the same one before running `claude`):

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
export ANTHROPIC_AUTH_TOKEN=unused
```

### 5. Run Claude Code

```bash
claude
```

That's it. Claude Code will now route all requests through oc-go-cc to OpenCode Go.

## How It Works

```
┌─────────────┐     Anthropic API      ┌─────────────┐     OpenAI API       ┌─────────────┐
│  Claude Code ├──────────────────────►│  oc-go-cc    ├────────────────────►│  OpenCode Go │
│  (CLI)       │  POST /v1/messages   │  (Proxy)     │  /chat/completions  │  (Upstream)  │
│              │◄──────────────────────┤              │◄────────────────────┤              │
└─────────────┘   Anthropic SSE        └─────────────┘   OpenAI SSE          └─────────────┘
```

1. Claude Code sends a request in [Anthropic Messages API](https://docs.anthropic.com/en/api/messages) format
2. oc-go-cc parses the request, counts tokens, and selects a model via routing rules
3. The request is transformed to [OpenAI Chat Completions](https://platform.openai.com/docs/api-reference/chat) format
4. The transformed request is sent to OpenCode Go's endpoint
5. The response (streaming or non-streaming) is transformed back to Anthropic format
6. Claude Code receives the response as if it came from Anthropic directly

### What Gets Transformed

| Anthropic                                                    | OpenAI                                  |
| ------------------------------------------------------------ | --------------------------------------- |
| `system` (string or array)                                   | `messages[0]` with `role: "system"`     |
| `content: [{"type":"text","text":"..."}]`                    | `content: "..."`                        |
| `tool_use` content blocks                                    | `tool_calls` array                      |
| `tool_result` content blocks                                 | `role: "tool"` messages                 |
| `thinking` content blocks                                    | `reasoning_content`                     |
| `stop_reason: "end_turn"`                                    | `finish_reason: "stop"`                 |
| `stop_reason: "tool_use"`                                    | `finish_reason: "tool_calls"`           |
| SSE `message_start` / `content_block_delta` / `message_stop` | SSE `role` / `delta.content` / `[DONE]` |

### DeepSeek V4 Thinking Mode

DeepSeek V4 Pro and Flash use the OpenAI-compatible `/chat/completions` endpoint through OpenCode Go. They support thinking mode and configurable reasoning effort.

For Claude Code and other agentic coding workflows, configure DeepSeek V4 models with:

```json
{
  "provider": "opencode-go",
  "model_id": "deepseek-v4-pro",
  "max_tokens": 8192,
  "reasoning_effort": "max",
  "thinking": {
    "type": "enabled"
  }
}
```

`oc-go-cc` forwards these fields to OpenCode Go as OpenAI Chat Completions parameters:

- `reasoning_effort`: controls DeepSeek V4 thinking effort (`high` or `max`)
- `thinking`: enables or disables DeepSeek V4 thinking mode

DeepSeek V4 thinking responses are returned as OpenAI `reasoning_content` and transformed back into Anthropic `thinking` blocks for Claude Code.

## Configuration

### Config File

Location: `~/.config/oc-go-cc/config.json`

Override with `OC_GO_CC_CONFIG` environment variable.

### Full Config Reference

```json
{
  "api_key": "${OC_GO_CC_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,

  "models": {
    "default": {
      "provider": "opencode-go",
      "model_id": "kimi-k2.6",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "background": {
      "provider": "opencode-go",
      "model_id": "qwen3.5-plus",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "think": {
      "provider": "opencode-go",
      "model_id": "glm-5.1",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m2.7",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 60000
    },
    "deepseek_v4_max": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.1,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": {
        "type": "enabled"
      }
    }
  },

  "fallbacks": {
    "default": [
      { "provider": "opencode-go", "model_id": "glm-5" },
      { "provider": "opencode-go", "model_id": "qwen3.6-plus" }
    ],
    "think": [{ "provider": "opencode-go", "model_id": "glm-5" }],
    "long_context": [{ "provider": "opencode-go", "model_id": "minimax-m2.5" }]
  },

  "opencode_go": {
    "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
    "timeout_ms": 300000
  },

  "logging": {
    "level": "info",
    "requests": true
  }
}
```

### Environment Variables

Environment variables override config file values. Config values also support `${VAR}` interpolation.

| Variable                | Description                                 | Default                                          |
| ----------------------- | ------------------------------------------- | ------------------------------------------------ |
| `OC_GO_CC_API_KEY`      | OpenCode Go API key (**required**)          | —                                                |
| `OC_GO_CC_CONFIG`       | Custom config file path                     | `~/.config/oc-go-cc/config.json`                 |
| `OC_GO_CC_HOST`         | Proxy listen host                           | `127.0.0.1`                                      |
| `OC_GO_CC_PORT`         | Proxy listen port                           | `3456`                                           |
| `OC_GO_CC_OPENCODE_URL` | OpenCode Go API endpoint                    | `https://opencode.ai/zen/go/v1/chat/completions` |
| `OC_GO_CC_LOG_LEVEL`    | Log level: `debug`, `info`, `warn`, `error` | `info`                                           |

### Model Routing

The proxy automatically detects the type of request and routes to the appropriate model based on context size and content analysis:

| Scenario         | Trigger                                             | Model        | Why                                             |
| ---------------- | --------------------------------------------------- | ------------ | ----------------------------------------------- |
| **Long Context** | >60K tokens                                         | MiniMax M2.7 | 1M context window vs 128-256K for others        |
| **Complex**      | "architect", "refactor", "complex" in system prompt | GLM-5.1      | Best reasoning & architectural understanding    |
| **Think**        | "think", "plan", "reason" in system prompt          | GLM-5        | Good reasoning, cheaper than GLM-5.1            |
| **Background**   | "read file", "grep", "list directory"               | Qwen3.5 Plus | Cheapest (~10K req/5hr), perfect for simple ops |
| **Default**      | Everything else                                     | Kimi K2.6    | Best balance of quality & cost (~1.8K req/5hr)  |

**📖 See [MODELS.md](MODELS.md) for detailed model capabilities, costs, and routing recommendations.**

DeepSeek V4 users can set any scenario model to `deepseek-v4-pro` or `deepseek-v4-flash`. For deterministic max thinking, add `reasoning_effort: "max"` and `thinking: {"type":"enabled"}` to that scenario's model config and fallback entries.

#### Routing in Detail:

| Scenario         | Trigger                                                                      | Config Key            | Default Model  |
| ---------------- | ---------------------------------------------------------------------------- | --------------------- | -------------- |
| **Default**      | Standard chat                                                                | `models.default`      | `kimi-k2.6`    |
| **Think**        | System prompt contains "think", "plan", "reason"; or thinking content blocks | `models.think`        | `glm-5.1`      |
| **Long Context** | Token count exceeds `context_threshold`                                      | `models.long_context` | `minimax-m2.7` |
| **Background**   | File read, directory list, grep patterns                                     | `models.background`   | `qwen3.5-plus` |

Routing priority: **Long Context** → **Think** → **Background** → **Default**

### Fallback Chains

When a model request fails (network error, rate limit, server error), the proxy tries the next model in the fallback chain:

```
Primary model → Fallback 1 → Fallback 2 → ... → Error (all failed)
```

Each model also has a **circuit breaker** that tracks consecutive failures. After 3 failures, the circuit opens and that model is skipped for 30 seconds, then tested again (half-open state).

### Available Models

See [MODELS.md](MODELS.md) for **detailed model capabilities, costs, and routing recommendations**.

Quick reference:

| Model ID       | Quality | Context | Cost (req/5hr) | Best For                              |
| -------------- | ------- | ------- | -------------- | ------------------------------------- |
| `glm-5.1`      | ★★★★★   | 200K    | ~880           | Complex architecture, difficult tasks |
| `glm-5`        | ★★★★☆   | 200K    | ~1,150         | High-quality coding, refactoring      |
| `kimi-k2.6`    | ★★★★★   | 256K    | ~1,850         | **Default** - best balance            |
| `kimi-k2.5`    | ★★★★☆   | 256K    | ~1,850         | Fallback - solid quality              |
| `mimo-v2-pro`  | ★★★★☆   | 128K    | ~1,290         | Code completion, generation           |
| `mimo-v2-omni` | ★★★☆☆   | 256K    | ~2,150         | Fast prototyping                      |
| `qwen3.6-plus` | ★★★☆☆   | 128K    | ~3,300         | Cost-effective general coding         |
| `minimax-m2.7` | ★★★☆☆   | **1M**  | ~3,400         | **Long context specialist**           |
| `minimax-m2.5` | ★★☆☆☆   | **1M**  | ~6,300         | Long context on a budget              |
| `deepseek-v4-pro` | ★★★★★ | **1M** | varies | Agentic coding, max thinking, long context |
| `deepseek-v4-flash` | ★★★★☆ | **1M** | varies | Fast agent tasks, background/subagent work |
| `qwen3.5-plus` | ★★☆☆☆   | 128K    | ~10,200        | **Cheapest** - background tasks       |

> **💡 Tip:** The cost column shows approximate requests per 5-hour block ($12). Qwen3.5 Plus gives you ~10x more requests than GLM-5.1!

> **⚠️ Important:** MiniMax M2.5 and M2.7 use the **Anthropic-compatible** `/v1/messages` endpoint natively. DeepSeek V4 Pro and Flash use the **OpenAI-compatible** `/chat/completions` endpoint and support `reasoning_effort` plus `thinking` for max thinking mode. See [MODELS.md](MODELS.md) for details.

## CLI Commands

```
oc-go-cc serve              Start the proxy server
oc-go-cc serve -b          Start in background (detached from terminal)
oc-go-cc serve --port 8080  Start on a custom port
oc-go-cc serve --config /path/to/config.json  Use a custom config
oc-go-cc stop               Stop the running proxy server
oc-go-cc status             Check if the proxy is running
oc-go-cc autostart enable   Enable auto-start on login
oc-go-cc autostart disable  Disable auto-start on login
oc-go-cc autostart status   Check autostart status
oc-go-cc init               Create default configuration file
oc-go-cc validate           Validate configuration file
oc-go-cc models             List available OpenCode Go models
oc-go-cc --version          Show version
```

## API Endpoints

The proxy exposes these endpoints that Claude Code expects:

| Method | Path                        | Description                           |
| ------ | --------------------------- | ------------------------------------- |
| `POST` | `/v1/messages`              | Main chat endpoint (Anthropic format) |
| `POST` | `/v1/messages/count_tokens` | Token counting                        |
| `GET`  | `/health`                   | Health check                          |

## Troubleshooting

### "invalid request body" Error

This means the proxy couldn't parse the request from Claude Code. Enable debug logging to see the raw request:

```json
{ "logging": { "level": "debug" } }
```

Or set the environment variable:

```bash
export OC_GO_CC_LOG_LEVEL=debug
```

### "all models failed" Error

All models in the fallback chain returned errors. Check:

1. Your API key is valid: `oc-go-cc validate`
2. You haven't exceeded your [usage limits](https://opencode.ai/auth)
3. The OpenCode Go service is reachable: `curl -H "Authorization: Bearer $OC_GO_CC_API_KEY" https://opencode.ai/zen/go/v1/models`

### Connection Refused

Make sure the proxy is running:

```bash
oc-go-cc status
```

And Claude Code is pointing to the right address:

```bash
echo $ANTHROPIC_BASE_URL  # Should be http://127.0.0.1:3456
```

### Streaming Not Working

The proxy transforms OpenAI SSE to Anthropic SSE in real-time. If streaming appears broken:

1. Set log level to `debug` to see the raw SSE chunks
2. Check that no proxy or firewall is buffering the connection
3. Try a non-streaming request first to verify the model works

### Debug Mode

For maximum logging, run with debug level:

```bash
OC_GO_CC_LOG_LEVEL=debug oc-go-cc serve
```

This logs:

- Raw Anthropic request body from Claude Code
- Transformed OpenAI request sent to OpenCode Go
- Raw OpenAI response received
- SSE stream events during streaming

## Architecture

```
cmd/oc-go-cc/main.go           CLI entry point (cobra commands)
internal/
├── config/
│   ├── config.go               Config types
│   └── loader.go               JSON loading, env overrides, ${VAR} interpolation
├── router/
│   ├── model_router.go         Model selection based on scenario
│   ├── scenarios.go            Scenario detection (default/think/long_context/background)
│   └── fallback.go            Fallback handler with circuit breaker
├── server/
│   └── server.go               HTTP server setup, graceful shutdown, PID management
├── handlers/
│   ├── messages.go             POST /v1/messages handler (streaming + non-streaming)
│   └── health.go               Health check and token counting endpoints
├── transformer/
│   ├── request.go              Anthropic → OpenAI request transformation
│   ├── response.go             OpenAI → Anthropic response transformation
│   └── stream.go               Real-time SSE stream transformation
├── client/
│   └── opencode.go             OpenCode Go HTTP client
└── token/
    └── counter.go              Tiktoken token counter (cl100k_base)
pkg/types/
├── anthropic.go                Anthropic API types (polymorphic system/content fields)
└── openai.go                   OpenAI API types
configs/
└── config.example.json         Example configuration
```

### Key Design Decisions

- **Polymorphic field handling**: Anthropic's `system` and `content` fields accept both strings and arrays. We use `json.RawMessage` with accessor methods (`SystemText()`, `ContentBlocks()`) to handle both formats correctly.
- **Real-time stream proxying**: SSE events are transformed in-flight, not buffered. This means Claude Code sees responses as they arrive from OpenCode Go.
- **Circuit breaker per model**: Each model gets its own circuit breaker. After 3 consecutive failures, the model is skipped for 30 seconds, then tested again.
- **Environment variable interpolation**: Config values like `"${OC_GO_CC_API_KEY}"` are resolved at load time, so you never need to put secrets in the config file.

## Development

```bash
# Build (version auto-detected from git)
make build

# Run in development mode
make run

# Run tests with race detector
make test

# Run go vet
make vet

# Clean build artifacts
make clean

# Install to $GOPATH/bin
make install

# Build cross-platform release binaries
make dist
```

## License

MIT
