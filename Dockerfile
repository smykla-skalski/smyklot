# syntax=docker/dockerfile:1@sha256:2780b5c3bab67f1f76c781860de469442999ed1a0d7992a5efdf2cffc0e3d769

# Use BUILDPLATFORM to run apk on native arch (avoids QEMU emulation issues)
# CA certificates are architecture-independent, so this is safe
FROM --platform=$BUILDPLATFORM alpine:3.23@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62 AS certs
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
