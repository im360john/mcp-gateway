# Start from the official MCP Gateway image
FROM ghcr.io/mcp-ecosystem/mcp-gateway/allinone:latest

# Set environment variables for IPv4 binding
ENV HOST=0.0.0.0
ENV BIND_ADDRESS=0.0.0.0
ENV APISERVER_HOST=127.0.0.1
ENV MCP_GATEWAY_HOST=127.0.0.1
ENV MOCK_SERVER_HOST=127.0.0.1
ENV NGINX_RESOLVER=127.0.0.11

# Set port configuration
ENV PORT=10000
ENV NGINX_PORT=10000
ENV APISERVER_PORT=5234
ENV MCP_GATEWAY_PORT=5235
ENV MCP_GATEWAY_WS_PORT=5335
ENV MOCK_SERVER_PORT=5236

# Set web paths
ENV ROOT_PATH=/
ENV BASE_PATH=/
ENV WEB_ROOT=/app/web
ENV HEALTH_CHECK_PATH=/api/health

# Create a simple Nginx configuration for IPv4 only
RUN echo "server { listen 127.0.0.1:5234; }" > /etc/nginx/conf.d/ipv4.conf && \
    echo "resolver 127.0.0.11 ipv6=off;" > /etc/nginx/conf.d/resolver.conf

# Create a health check endpoint
RUN mkdir -p /app/health && \
    echo '{"status":"ok"}' > /app/health/index.json && \
    echo 'location /health { root /app; try_files /health/index.json =404; }' > /etc/nginx/conf.d/health.conf

# Expose the port
EXPOSE 10000

# Use the default entrypoint and command
ENTRYPOINT ["/entrypoint.sh"]
CMD ["supervisord", "-c", "/etc/supervisord.conf"]
