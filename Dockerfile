# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89

# Use BUILDPLATFORM to run apk on native arch (avoids QEMU emulation issues)
# CA certificates are architecture-independent, so this is safe
FROM --platform=$BUILDPLATFORM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS certs
RUN apk --no-cache add ca-certificates

FROM scratch

ARG TARGETPLATFORM

LABEL org.opencontainers.image.source="https://github.com/smykla-skalski/smyklot"
LABEL org.opencontainers.image.description="Automated PR approvals and merges based on CODEOWNERS"
LABEL org.opencontainers.image.licenses="MIT"

# Copy CA certificates from alpine
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy pre-built binary from GoReleaser
COPY ${TARGETPLATFORM}/smyklot /smyklot

ENTRYPOINT ["/smyklot"]
