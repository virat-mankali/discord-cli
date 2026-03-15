# 🎮 discocli — Discord CLI + MCP Server

Interact with Discord from your terminal. Built for developers and AI agents.

```bash
discocli sync --follow          # real-time message capture
discocli search "deployment"    # offline full-text search
discocli send --to "#general" --text "Build passed ✅"
discocli serve                  # start MCP server for AI agents
```

## Install

### Homebrew (macOS & Linux)

```bash
brew install virat-mankali/tap/discocli
```

### Build from source

```bash
git clone https://github.com/virat-mankali/discord-cli
cd discord-cli
go build -o discocli ./cmd/discocli/
```

## Quick Start

```bash
# 1. Authenticate (bot token or user token)
discocli auth

# 2. Sync messages
discocli sync

# 3. Search offline
discocli search "standup notes"

# 4. Send a message
discocli send --to "#general" --text "Hello from the terminal!"
```

## Commands

| Command | Description |
|---------|-------------|
| `discocli auth` | Authenticate with Discord (bot or user token) |
| `discocli sync` | Sync message history to local SQLite database |
| `discocli sync --follow` | Real-time sync via Discord Gateway |
| `discocli search <query>` | Full-text search across synced messages |
| `discocli send --to <target> --text <msg>` | Send message to channel, DM, or thread |
| `discocli guilds` | List all servers you're in |
| `discocli channels --guild <name>` | List channels in a server |
| `discocli serve` | Start MCP server for AI agents |

## MCP Server (AI Agents)

Add to your `mcp.json`:

```json
{
  "mcpServers": {
    "discocli": {
      "command": "discocli",
      "args": ["serve"],
      "env": {},
      "disabled": false,
      "autoApprove": [
        "search_messages",
        "list_guilds",
        "list_channels",
        "get_sync_status"
      ]
    }
  }
}
```

### Available MCP Tools

| Tool | Description | Auto-approve? |
|------|-------------|---------------|
| `search_messages` | Full-text search across synced messages | ✅ |
| `list_guilds` | List all Discord servers | ✅ |
| `list_channels` | List channels in a server | ✅ |
| `get_sync_status` | Show sync status and message counts | ✅ |
| `send_message` | Send a message to a channel | ❌ |
| `sync_channel` | Sync a channel's history | ❌ |

## How It Works

- Messages are synced from Discord into a local SQLite database with FTS5 full-text search
- All searches are offline — fast and private
- Incremental sync only fetches new messages on subsequent runs
- The MCP server exposes the same functionality over stdio for AI agents
- Pure Go binary, no CGO — works on all platforms

## Token Setup

### Bot Token (Recommended)

1. Go to [Discord Developer Portal](https://discord.com/developers/applications)
2. Create application → Bot → Reset Token
3. Enable **Message Content Intent** under Privileged Gateway Intents
4. Invite bot to your server with `Read Messages`, `Send Messages`, `Read Message History`, `Attach Files` permissions

### User Token (Personal Use)

1. Open Discord in browser
2. DevTools → Network → any API request → Authorization header

## License

MIT
