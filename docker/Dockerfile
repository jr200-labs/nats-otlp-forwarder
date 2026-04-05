ARG GOVERSION=1.26
FROM golang:${GOVERSION}-alpine AS builder

ARG BUILD_ARCH
ARG BUILD_OS

RUN apk update && apk add --no-cache git bash make

WORKDIR /usr/src/app

# Pre-download deps
COPY go.sum go.mod ./
RUN go mod download

COPY . .

RUN make build && \
    ln -s /usr/src/app/build/nats-otlp-forwarder-${BUILD_OS}-${BUILD_ARCH} /usr/local/bin/nats-otlp-forwarder

# Runtime stage
FROM alpine:3

LABEL org.opencontainers.image.source="https://github.com/jr200/nats-otlp-forwarder"
LABEL org.opencontainers.image.description="nats-otlp-forwarder runtime"

RUN apk add --no-cache ca-certificates wget && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

COPY ./docker/docker-entrypoint.sh /docker-entrypoint.sh
COPY ./docker/config /config/
COPY --from=builder /usr/local/bin/nats-otlp-forwarder /usr/local/bin/nats-otlp-forwarder

USER appuser

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/docker-entrypoint.sh"]
