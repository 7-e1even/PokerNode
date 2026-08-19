# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:24-alpine AS build
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    corepack enable && pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM nginx:1.30.4-alpine
COPY web/nginx.conf /etc/nginx/nginx.conf
COPY --from=build --chown=nginx:nginx /src/web/dist /usr/share/nginx/html
USER nginx
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT []
CMD ["nginx", "-g", "daemon off;"]
