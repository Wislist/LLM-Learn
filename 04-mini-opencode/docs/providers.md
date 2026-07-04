# Providers

`mini-opencode` currently supports two provider modes:

- `echo`: local fallback provider used for development
- `deepseek` / `openai-compatible`: OpenAI-compatible chat completions API

## Echo

If `config.json` is missing, the app uses `echo` automatically.

```json
{
  "provider": {
    "name": "echo"
  }
}
```

## DeepSeek

Create `config.json` in `04-mini-opencode`:

```json
{
  "provider": {
    "name": "deepseek",
    "base_url": "https://api.deepseek.com",
    "model": "deepseek-chat",
    "api_key_env": "DEEPSEEK_API_KEY"
  }
}
```

Then run:

```bash
export DEEPSEEK_API_KEY="your-key"
go run ./cmd/mini-opencode
```

The provider uses the OpenAI-compatible `/chat/completions` endpoint and supports tool call conversion. Streaming is not implemented yet.

