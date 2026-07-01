FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY go.mod ./
# Full source for what we build
COPY pkg/product/ pkg/product/
COPY cmd/dupli1-product/ cmd/dupli1-product/
# Manifests-only for other workspace modules (go.work requires all use entries to resolve)
COPY pkg/auth/go.mod pkg/auth/go.sum pkg/auth/
COPY pkg/inventory/go.mod pkg/inventory/
COPY pkg/notification/go.mod pkg/notification/
COPY pkg/order/go.mod pkg/order/
COPY cmd/dupli1-auth/go.mod cmd/dupli1-auth/
COPY cmd/dupli1-inventory/go.mod cmd/dupli1-inventory/
COPY cmd/dupli1-notification/go.mod cmd/dupli1-notification/
COPY cmd/dupli1-order/go.mod cmd/dupli1-order/
RUN go build -o /dupli1-product ./cmd/dupli1-product

FROM alpine:3.21
RUN adduser -D -u 1000 app
USER app
COPY --from=builder /dupli1-product /dupli1-product
ENTRYPOINT ["/dupli1-product"]
