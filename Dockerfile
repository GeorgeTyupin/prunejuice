# syntax=docker/dockerfile:1

# ---- build stage ----------------------------------------------------------
FROM golang:1.23-alpine AS build
WORKDIR /src

# Cache modules first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=docker
# Static binary (CGO off) so it runs on any small base image.
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/prunejuice ./cmd/prunejuice

# ---- runtime stage --------------------------------------------------------
FROM alpine:3.20

# ca-certificates: needed for HTTPS calls to the Telegram API.
# util-linux:      provides `nsenter`, used to run cleanup commands in the
#                  HOST's namespaces (see config.docker.yaml / README).
RUN apk add --no-cache ca-certificates util-linux

COPY --from=build /out/prunejuice /usr/local/bin/prunejuice

# Runs continuously with an internal ticker (check_interval). Override the
# config path or flags via `command:` in docker-compose if needed.
ENTRYPOINT ["/usr/local/bin/prunejuice"]
CMD ["-config", "/etc/prunejuice/config.yaml", "-daemon"]
