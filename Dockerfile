# Builder runs natively on the build host and cross-compiles for each target
# platform (no QEMU emulation needed for a CGO-free Go build).
FROM --platform=$BUILDPLATFORM golang:1.27.4@sha256:08bcb99a38f85a265f9a93d1d3323e2440eec333380f3ea26ee0dec9264bb41c AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY src/go.mod .
COPY src/go.sum .
RUN go mod download

COPY src/ .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o truenas-exporter "./cmd"

FROM scratch

# CA bundle for TrueNAS instances with a CA-signed certificate (unused when
# TRUENAS_TLS_FINGERPRINT pins a self-signed one).
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/truenas-exporter /truenas-exporter

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/truenas-exporter"]
