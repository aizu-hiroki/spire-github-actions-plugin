FROM golang:1.21-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o /nodeattestor-agent ./cmd/nodeattestor-agent/

FROM alpine:3.19

ARG SPIRE_VERSION=1.9.6

RUN apk add --no-cache bash curl && \
    curl -sSfL "https://github.com/spiffe/spire/releases/download/v${SPIRE_VERSION}/spire-${SPIRE_VERSION}-linux-amd64-musl.tar.gz" \
      -o /tmp/spire.tar.gz && \
    tar xzf /tmp/spire.tar.gz -C /tmp/ && \
    cp /tmp/spire-${SPIRE_VERSION}/bin/spire-agent /usr/local/bin/ && \
    rm -rf /tmp/spire*

COPY --from=builder /nodeattestor-agent /usr/local/bin/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
