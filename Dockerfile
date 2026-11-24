# Build backend
FROM golang:1.27-alpine AS backend-builder
RUN apk add --no-cache git
WORKDIR /build/code

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
go mod download

RUN --mount=type=cache,target=/go/pkg/mod \
go build -o /build/goose github.com/pressly/goose/v3/cmd/goose

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /build/app .

# Build frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /build/frontend
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npx start-hexlet-url-shortener-frontend --build || npm run build

# Runtime
FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache bash caddy
RUN addgroup -S app && adduser -S -G app app
RUN chown -R app:app /app

COPY --from=backend-builder /build/app /app/bin/app
COPY --from=backend-builder /build/goose /usr/local/bin/goose
COPY --from=backend-builder build/code/db/migrations /app/db/migrations
COPY bin/run.sh /app/bin/run.sh
RUN chmod +x /app/bin/run.sh

# Статика фронтенда + конфиг Caddy
COPY --from=frontend-builder /build/frontend/dist /srv/frontend
COPY Caddyfile /etc/caddy/Caddyfile

USER app

EXPOSE 8080

CMD ["/app/bin/run.sh"]
