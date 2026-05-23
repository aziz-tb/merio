# Multi-stage build for the libredesk binary.
#
# Stages:
#   1. frontend — builds frontend/dist/{main,widget} via pnpm
#   2. backend  — compiles the Go binary and embeds assets via stuffbin
#   3. runtime  — minimal alpine image carrying just the stuffed binary

# ----- Stage 1: frontend ------------------------------------------------
FROM node:20-alpine AS frontend

RUN corepack enable && corepack prepare pnpm@9.15.3 --activate

WORKDIR /build/frontend

COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY frontend/ ./
RUN pnpm build:main && pnpm build:widget

# ----- Stage 2: backend -------------------------------------------------
FROM golang:1.25-alpine AS backend

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

RUN go install github.com/knadh/stuffbin/...@v1.3.0

COPY . .

COPY --from=frontend /build/frontend/dist ./frontend/dist

ARG VERSION=dev
ARG GIT_COMMIT=unknown

RUN CGO_ENABLED=0 go build -a \
    -ldflags="-s -w \
        -X 'main.buildString=${VERSION} (${GIT_COMMIT})' \
        -X 'main.versionString=${VERSION}' \
        -X 'github.com/abhinavxd/libredesk/internal/version.Version=${VERSION}'" \
    -o /out/libredesk \
    cmd/*.go

RUN stuffbin -a stuff -in /out/libredesk -out /out/libredesk \
    frontend/dist i18n schema.sql static

# ----- Stage 3: runtime -------------------------------------------------
FROM alpine:3.18

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /libredesk

COPY --from=backend /out/libredesk .
COPY config.sample.toml config.toml

EXPOSE 9000
CMD ["./libredesk"]