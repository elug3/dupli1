FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY go.mod ./
COPY pkg/auth/go.mod pkg/auth/go.sum pkg/auth/
COPY pkg/inventory/go.mod pkg/inventory/
COPY pkg/notification/go.mod pkg/notification/
COPY pkg/order/go.mod pkg/order/
COPY pkg/product/go.mod pkg/product/go.sum pkg/product/
COPY cmd/dupli1-auth/go.mod cmd/dupli1-auth/
COPY cmd/dupli1-inventory/go.mod cmd/dupli1-inventory/
COPY cmd/dupli1-notification/go.mod cmd/dupli1-notification/
COPY cmd/dupli1-order/go.mod cmd/dupli1-order/
COPY cmd/dupli1-product/go.mod cmd/dupli1-product/
# Full source for what we build
COPY pkg/inventory/ pkg/inventory/
COPY cmd/dupli1-inventory/ cmd/dupli1-inventory/
RUN go build -o /dupli1-inventory ./cmd/dupli1-inventory

FROM alpine:3.21
RUN adduser -D -u 1000 app
USER app
COPY --from=builder /dupli1-inventory /dupli1-inventory
ENTRYPOINT ["/dupli1-inventory"]
