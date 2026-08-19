# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS backend
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /pokernode ./cmd/pokernode && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /pokernode-mcp ./cmd/pokernode-mcp

FROM alpine:3.22
RUN adduser -D -u 10001 pokernode && mkdir -p /app && chown -R pokernode:pokernode /app
WORKDIR /app
COPY --from=backend /pokernode /app/pokernode
COPY --from=backend /pokernode-mcp /app/pokernode-mcp
USER pokernode
ENV POKERNODE_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/readyz >/dev/null || exit 1
CMD ["/app/pokernode"]
