FROM ghcr.io/mcp-ecosystem/mcp-gateway/allinone:latest

# Create a startup script that handles port conflicts
COPY start.sh /app/start.sh
RUN chmod +x /app/start.sh

EXPOSE $PORT

CMD ["/start.sh"]
