# Overview

This MCP server is supposed to be running behind the [Pomerium](https://github.com/pomerium/pomerium) application gateway, that provides TLS, authentication with any OIDC compliant identity provider and authorization policies.

It registers a single `whoami` MCP tool that uses the identity assertion header that Pomerium passes and responds back with the user information.
