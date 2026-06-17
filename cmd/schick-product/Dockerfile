FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY go.mod ./
# Full source for what we build
COPY pkg/product/ pkg/product/
COPY cmd/schick-product/ cmd/schick-product/
# Manifests-only for other workspace modules (go.work requires all use entries to resolve)
COPY pkg/auth/go.mod pkg/auth/go.sum pkg/auth/
COPY pkg/inventory/go.mod pkg/inventory/
COPY pkg/notification/go.mod pkg/notification/
COPY pkg/order/go.mod pkg/order/
COPY cmd/schick-auth/go.mod cmd/schick-auth/
COPY cmd/schick-inventory/go.mod cmd/schick-inventory/
COPY cmd/schick-notification/go.mod cmd/schick-notification/
COPY cmd/schick-order/go.mod cmd/schick-order/
RUN go build -o /schick-product ./cmd/schick-product

FROM alpine:3.21
RUN adduser -D -u 1000 app
USER app
COPY --from=builder /schick-product /schick-product
ENTRYPOINT ["/schick-product"]
