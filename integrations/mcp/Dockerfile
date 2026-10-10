FROM python:3.13-slim AS build
COPY --from=ghcr.io/astral-sh/uv:0.11.8 /uv /usr/local/bin/uv
WORKDIR /app
COPY integrations/mcp/pyproject.toml integrations/mcp/uv.lock ./
COPY integrations/mcp/README.md ./
COPY integrations/mcp/src ./src
# Build the MCP app before this image; Docker must not silently omit its resource.
RUN test -f src/ding_mcp/assets/workspace.html && uv sync --frozen --no-dev --no-editable

FROM python:3.13-slim
RUN groupadd --gid 10001 ding && useradd --uid 10001 --gid ding --no-create-home ding
WORKDIR /app
COPY --from=build /app/.venv /app/.venv
USER ding
EXPOSE 7677
ENTRYPOINT ["/app/.venv/bin/ding-mcp"]
CMD ["serve", "--transport", "http", "--http-config", "/run/ding/http.json", "--host", "0.0.0.0"]
