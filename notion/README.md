# Overview

This is a Notion MCP server that supports OAuth2 authentication for the current user.

## Prerequisites

This is not a standalone server. It requires [Pomerium](https://www.pomerium.com/) to handle TLS, traffic proxying, user authentication and authorization, and OAuth flows. See the [quick start guide](/README.md) for more information.

## Setup

1. Create an OAuth2 Notion Client (called "External Integration") at [Notion Integrations](https://www.notion.so/profile/integrations).
2. In the **Basic Information** tab, set **OAuth domains & URIs** to:

   ```
   https://notion.YOUR-DOMAIN/.pomerium/mcp/oauth/callback
   ```
