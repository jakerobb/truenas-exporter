# Builder runs natively on the build host and cross-compiles for each target
# platform (no QEMU emulation needed for a CGO-free Go build).
FROM --platform=$BUILDPLATFORM golang:1.27.3@sha256:9048d5e7751706200eb1719c7dae0436950d362f29a1da91fa05e5c63867c60f AS builder

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
