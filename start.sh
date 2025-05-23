#!/bin/bash

# Set default PORT if not provided
export PORT=${PORT:-10000}

# Configure MCP Gateway to use different internal ports
export APISERVER_PORT=8081
export GATEWAY_PORT=8082
export MCP_SSE_PORT=8083
export MCP_HTTP_PORT=8084

# Start the application with port mapping
exec /app/mcp-gateway
