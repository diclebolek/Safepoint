# =============================================================================
# Bu dosya ne ise yarar?
#   Safepoint operator container image'ini uretir (cmd/main.go -> /manager).
#   Ornek: docker build -t backup-operator:dev .
# =============================================================================

FROM golang:1.27-alpine AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /manager ./cmd

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /manager /manager
USER 65532:65532
ENTRYPOINT ["/manager"]
