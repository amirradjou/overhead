# One image for every overhead command (README: Infra): the Go binary plus the screen's static files.
#   docker build -t overhead .
#   docker run --rm overhead poll        # or enrich, live -web /web, history -s3 s3://..., headsup

FROM node:22-alpine AS web
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@12 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN NEXT_TELEMETRY_DISABLED=1 pnpm build

FROM golang:1.26-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/amirradjou/overhead/internal/buildinfo.Version=${VERSION}" \
    -o /overhead ./cmd/overhead
# The data directory, owned by distroless's nonroot user: a fresh named volume takes its owner from here.
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /overhead /overhead
COPY --from=web /src/web/out /web
COPY --from=go --chown=65532:65532 /out/data /data
ENV OVERHEAD_DATA=/data
USER nonroot
ENTRYPOINT ["/overhead"]
CMD ["live", "-web", "/web"]
