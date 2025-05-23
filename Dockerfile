FROM ghcr.io/mcp-ecosystem/mcp-gateway/allinone:latest

# Create a startup script that handles port conflicts
COPY start.sh /app/mcp-gateway/start.sh
RUN chmod +x /app/mcp-gateway/start.sh

EXPOSE $PORT

CMD ["/app/mcp-gateway/start.sh"]
