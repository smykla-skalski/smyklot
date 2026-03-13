# syntax=docker/dockerfile:1@sha256:b6afd42430b15f2d2a4c5a02b919e98a525b785b1aaff16747d2f623364e39b6

# Use BUILDPLATFORM to run apk on native arch (avoids QEMU emulation issues)
# CA certificates are architecture-independent, so this is safe
FROM --platform=$BUILDPLATFORM alpine:3.23@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659 AS certs
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
