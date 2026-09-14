# syntax=docker/dockerfile:1

# ---------- build: Go API ----------
FROM golang:1.25-bookworm AS gobuild
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/server

# ---------- build: Vue/TypeScript production bundle ----------
FROM node:20-bookworm AS webbuild
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---------- target: api ----------
FROM debian:bookworm-slim AS api
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=gobuild /out/api /app/api
RUN mkdir -p /data
ENV ADDR=":8080" DB_DIR="/data"
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --retries=10 \
    CMD curl -fsS http://localhost:8080/api/health || exit 1
ENTRYPOINT ["/app/api"]

# ---------- target: web ----------
FROM nginx:1.27-bookworm AS web
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=webbuild /src/web/dist /usr/share/nginx/html
EXPOSE 80
HEALTHCHECK --interval=5s --timeout=3s --retries=10 \
    CMD curl -fsS http://localhost:80/ >/dev/null || exit 1

# ---------- target: verify (one-shot) ----------
FROM golang:1.25-bookworm AS verify
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl gnupg \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY . .
RUN chmod +x scripts/*.sh
WORKDIR /src/web
RUN npm ci && npx playwright install --with-deps chromium
ENTRYPOINT ["/src/scripts/verify.sh"]
