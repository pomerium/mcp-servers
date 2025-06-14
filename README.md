# Overview

This repository contains a collection of reference MCP Streaming HTTP servers.

:::note
This is not an official Pomerium product.
:::

# Prerequisites

1. Linux or macOS host
2. Docker and Docker Compose
3. Your machine must have port 443 exposed to the internet so it can acquire TLS certificates from Let's Encrypt - public MCP Client cannot work with self-signed certs.

# Quick Start

1. Choose your Pomerium installation method below.
2. Enable the relevant MCP server(s) by providing the appropriate environment variables.
   - [Notion](./notion/README.md): A specially tailored Notion MCP server that uses Notion OAuth for the current user and specifically implements [OpenAI Deep Researcher requirements](https://platform.openai.com/docs/mcp).

## Docker Compose Example

```docker
services:
  postgres:
    image: postgres:17
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: pomerium
      POSTGRES_HOST_AUTH_METHOD: trust
    ports:
      - 5432:5432
    volumes:
      - postgres-data:/var/lib/postgresql/data

  pomerium:
    image: pomerium/pomerium:main
    ports:
      - "443:443"
      - "80:80"
    volumes:
      - ./config.yaml:/pomerium/config.yaml
      - pomerium-autocert:/data/autocert

  mcp-servers:
    image: pomerium/mcp-servers:latest
    expose:
      - 8080
    environment:
      # See relevant sub-directories for required environment variables if any

volumes:
  postgres-data:
  pomerium-autocert:
```
