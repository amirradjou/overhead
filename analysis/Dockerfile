# overhead-stats: the facts job (docs/SPEC.md: Where it runs). docker compose builds it as overhead-stats:local.
FROM ghcr.io/astral-sh/uv:0.12.15-python3.13-trixie-slim
WORKDIR /app
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy UV_NO_DEV=1
COPY pyproject.toml uv.lock .python-version ./
RUN uv sync --frozen --no-install-project
COPY src ./src
RUN uv sync --frozen
ENTRYPOINT ["uv", "run", "--frozen", "overhead-stats"]
CMD ["facts", "--every", "10"]
