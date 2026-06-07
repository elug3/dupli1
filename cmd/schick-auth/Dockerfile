FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY go.mod ./
# Full source for what we build
COPY pkg/auth/ pkg/auth/
COPY cmd/schick-auth/ cmd/schick-auth/
# Manifests-only for other workspace modules (go.work requires all use entries to resolve)
COPY pkg/product/go.mod pkg/product/go.sum pkg/product/
COPY cmd/schick-product/go.mod cmd/schick-product/
RUN go build -o /schick-auth ./cmd/schick-auth

FROM alpine:3.21
RUN adduser -D -u 1000 app
USER app
COPY --from=builder /schick-auth /schick-auth
ENTRYPOINT ["/schick-auth"]
