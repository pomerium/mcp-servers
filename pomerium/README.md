# Pomerium MCP Server

An MCP server that exposes the Pomerium [ConfigService](https://www.pomerium.com/docs) API as tools, allowing LLMs and AI agents to manage a Pomerium instance — routes, policies, settings, key pairs, and service accounts.

All tools are auto-discovered from the Pomerium SDK's protobuf definitions at runtime. Adding new ConfigService methods requires zero code changes.

## Configuration

The server requires two environment variables:

| Variable | Required | Description |
|---|---|---|
| `POMERIUM_API_URL` | Yes | Base URL of the Pomerium API (varies by deployment mode — see below) |
| `POMERIUM_API_TOKEN` | Yes | Authentication credential — either a **service account token** or a **base64-encoded shared secret** |
| `POMERIUM_TLS_INSECURE_SKIP_VERIFY` | No | Skip TLS certificate verification (default `false`) |

The server auto-detects the Pomerium deployment type (Core, Enterprise, or Zero) via `GetServerInfo` and applies the correct authentication headers automatically.

### Pomerium Core

For self-hosted [Pomerium Core](https://www.pomerium.com/docs) (open-source), point `POMERIUM_API_URL` at the gRPC address and use the base64-encoded **shared secret** as the token:

```bash
export POMERIUM_API_URL="http://localhost:5443"
export POMERIUM_API_TOKEN="<base64-encoded-shared-secret>"
```

Use the same `SHARED_SECRET` value from your Pomerium Core configuration. The server automatically generates the required bootstrap and gRPC JWTs from the shared secret.

> **Note:** Service account tokens are not available in Core mode — only the shared secret method works.

### Pomerium Enterprise

For [Pomerium Enterprise](https://www.pomerium.com/docs/deploy/enterprise), point `POMERIUM_API_URL` at the Enterprise Console API endpoint.

**Option 1: Service Account Token** (Recommended)

Create a [Service Account](https://www.pomerium.com/docs/capabilities/service-accounts) in the Enterprise Console and use the generated token:

```bash
export POMERIUM_API_URL="https://console-api.your-domain.com"
export POMERIUM_API_TOKEN="eyJhbGciOiJIUzI1NiIs..."
```

The Pomerium route for the Console API must grant access to the service account user in its policy.

**Option 2: Shared Secret (Bootstrap)**

Use the base64-encoded shared secret for bootstrapping — useful when configuring Pomerium Enterprise as part of the installation process, before you have access to the UI to create a service account:

```bash
export POMERIUM_API_URL="https://console-api.your-domain.com"
export POMERIUM_API_TOKEN="<base64-encoded-shared-secret>"
```

This requires `BOOTSTRAP_SERVICE_ACCOUNT=true` in your Enterprise Console configuration. The Console API route policy must allow the bootstrap user:

```yaml
policy:
  - allow:
      or:
        - user:
            is: "bootstrap-014e587b-3f4b-4fcf-90a9-f6ecdf8154af.pomerium"
```

### Pomerium Zero

For [Pomerium Zero](https://www.pomerium.com/docs/deploy/pomerium-zero) (managed SaaS), point `POMERIUM_API_URL` at the Zero console and use an API User token:

```bash
export POMERIUM_API_URL="https://console.pomerium.app"
export POMERIUM_API_TOKEN="<zero-api-user-token>"
```

Create an API User in the Pomerium Zero dashboard to obtain the token.

### Configuration Summary

| | Core | Enterprise | Zero |
|---|---|---|---|
| **`POMERIUM_API_URL`** | `http://<host>:5443` (gRPC port) | `https://console-api.<domain>` | `https://console.pomerium.app` |
| **Credential** | Shared secret only | Service account token or shared secret | API User token only |
| **Prerequisites** | gRPC port accessible | Console API route accessible | API User created in dashboard |

## Running

### Stdio Transport (for Claude Code, Cursor, etc.)

```bash
mcp-servers stdio pomerium
```

Or with environment variable:

```bash
SERVER=pomerium mcp-servers stdio
```

### Claude Code Configuration

Add to your Claude Code MCP settings (`.mcp.json`):

```json
{
  "mcpServers": {
    "pomerium": {
      "command": "mcp-servers",
      "args": ["stdio", "pomerium"],
      "env": {
        "POMERIUM_API_URL": "http://localhost:5443",
        "POMERIUM_API_TOKEN": "your-shared-secret-or-service-account-token"
      }
    }
  }
}
```

### HTTP Transport (multi-server mode)

When running the combined HTTP server, the Pomerium MCP server is available at the `/pomerium` path:

```bash
export POMERIUM_API_URL="https://pomerium.example.com:443"
export POMERIUM_API_TOKEN="your-token-or-shared-secret"
mcp-servers serve
```

### Docker Compose

```yaml
services:
  mcp-servers:
    image: pomerium/mcp-servers:main
    expose:
      - 8080
    environment:
      POMERIUM_API_URL: "http://pomerium:5443"
      POMERIUM_API_TOKEN: "${SHARED_SECRET}"
```

## Available Tools

The server exposes all ConfigService methods as MCP tools (23 total). Tool names are derived from method names using `snake_case`:

### Routes
| Tool | Description | Annotations |
|---|---|---|
| `list_routes` | List routes | Read-only |
| `get_route` | Get a route by ID | Read-only |
| `create_route` | Create a new route | Non-destructive |
| `update_route` | Update an existing route | Idempotent |
| `delete_route` | Delete a route | Destructive, Idempotent |

### Policies
| Tool | Description | Annotations |
|---|---|---|
| `list_policies` | List policies | Read-only |
| `get_policy` | Get a policy by ID | Read-only |
| `create_policy` | Create a new policy | Non-destructive |
| `update_policy` | Update an existing policy | Idempotent |
| `delete_policy` | Delete a policy | Destructive, Idempotent |

### Settings
| Tool | Description | Annotations |
|---|---|---|
| `list_settings` | List settings | Read-only |
| `get_settings` | Get settings | Read-only |
| `update_settings` | Update settings | Idempotent |

### Key Pairs (Certificates)
| Tool | Description | Annotations |
|---|---|---|
| `list_key_pairs` | List key pairs | Read-only |
| `get_key_pair` | Get a key pair by ID | Read-only |
| `create_key_pair` | Create a new key pair | Non-destructive |
| `update_key_pair` | Update an existing key pair | Idempotent |
| `delete_key_pair` | Delete a key pair | Destructive, Idempotent |

### Service Accounts
| Tool | Description | Annotations |
|---|---|---|
| `list_service_accounts` | List service accounts | Read-only |
| `get_service_account` | Get a service account by ID | Read-only |
| `create_service_account` | Create a new service account | Non-destructive |
| `update_service_account` | Update a service account | Idempotent |
| `delete_service_account` | Delete a service account | Destructive, Idempotent |

### Tool Annotations

MCP tool annotations are inferred from method naming conventions:

- **`Get*` / `List*`** → `readOnlyHint: true` — safe to call without side effects
- **`Create*`** → `destructiveHint: false` — creates new resources
- **`Update*`** → `destructiveHint: false, idempotentHint: true` — modifies existing resources
- **`Delete*`** → `destructiveHint: true, idempotentHint: true` — removes resources

Both input and output JSON schemas are generated from protobuf message definitions, so LLMs can understand the expected request/response structure for each tool.

## How It Works

The server uses protobuf reflection to walk the Pomerium SDK's `File_config_proto` descriptor at runtime:

1. Discovers all services and methods in the ConfigService
2. Generates JSON Schema for each method's input and output messages
3. Infers MCP tool annotations from method naming conventions
4. Registers each method as an MCP tool
5. Tool calls are forwarded as [Connect](https://connectrpc.com/) unary JSON requests to the Pomerium API

This means new ConfigService methods are automatically available as MCP tools when the SDK dependency is updated — no code changes needed.
