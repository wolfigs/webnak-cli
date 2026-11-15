# Webnak for ChatGPT

The package uploaded to the OpenAI plugin dashboard (platform.openai.com/plugins)
to list Webnak in ChatGPT. It points ChatGPT at Webnak's remote MCP server,
`https://api.cloud.wolfigs.com/mcp`, which signs users in with OAuth (dynamic
client registration).

Build the ZIP to upload:

```sh
scripts/openai-package.sh   # writes dist/webnak-chatgpt.zip
```

Test cases, the demo video and reviewer credentials are entered in the
dashboard, not here. Changes to the MCP server's tools need no new upload;
changes to this folder (listing text, icons) do.
