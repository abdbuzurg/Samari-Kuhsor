# The Go API.
#
# Multi-stage: the runtime image carries a static binary and the migrations, and
# nothing else — no toolchain, no source, no shell package manager. A smaller
# attack surface on a single server in Dushanbe with no security team behind it.

FROM golang:1.26-alpine AS build
WORKDIR /src

# Dependencies first, so a code change does not re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO off: the binary must run on a scratch-like base with no libc.
# -trimpath strips local paths out of stack traces, which would otherwise leak
# the build machine's directory layout into production logs.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w" \
      -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w" \
      -o /out/seed ./cmd/seed
# Website-analytics maintenance. The API runs it daily in-process; this is for
# catching up after downtime and for verifying retention by hand (D12).
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w" \
      -o /out/analytics ./cmd/analytics

FROM alpine:3.21 AS runtime
# ca-certificates only. The API makes no outbound TLS calls today, but a future
# one failing with an unexplained x509 error is a bad hour to spend.
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S samari && adduser -S -G samari samari

WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/seed /app/seed
COPY --from=build /out/analytics /app/analytics
# Migrations ship WITH the binary rather than being applied from a workstation.
# A deploy that can migrate itself is one that cannot be half-applied because
# somebody's laptop lost its VPN.
COPY --from=build /src/migrations /app/migrations

USER samari
ENV APP_ENV=production
EXPOSE 8080

# No host port is published for this service in production compose — it is
# reachable only by the two Next.js containers on the compose network (I8/I18).
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- --header="X-Service-Key: $SERVICE_KEY" http://127.0.0.1:8080/api/v1/health || exit 1

ENTRYPOINT ["/app/api"]
