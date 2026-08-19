FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.25-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /pokernode ./cmd/pokernode

FROM alpine:3.22
RUN adduser -D -u 10001 pokernode && mkdir -p /app/web /data && chown -R pokernode:pokernode /app /data
WORKDIR /app
COPY --from=backend /pokernode /app/pokernode
COPY --from=web /src/web/dist /app/web/dist
USER pokernode
ENV POKERNODE_ADDR=:8080
EXPOSE 8080
CMD ["/app/pokernode"]
