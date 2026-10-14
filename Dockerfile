# Builder runs natively on the build host and cross-compiles for each target
# platform (no QEMU emulation needed for a CGO-free Go build).
FROM --platform=$BUILDPLATFORM golang:1.27.2@sha256:820153096fda4de6ac1b7194d12e416b7dd8f4e9a33d17e0345b111b945162d8 AS builder

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
