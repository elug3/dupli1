FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY go.mod ./
COPY pkg/auth/go.mod pkg/auth/go.sum pkg/auth/
COPY pkg/inventory/go.mod pkg/inventory/
COPY pkg/notification/go.mod pkg/notification/
COPY pkg/order/go.mod pkg/order/
COPY pkg/product/go.mod pkg/product/go.sum pkg/product/
COPY cmd/schick-auth/go.mod cmd/schick-auth/
COPY cmd/schick-inventory/go.mod cmd/schick-inventory/
COPY cmd/schick-notification/go.mod cmd/schick-notification/
COPY cmd/schick-order/go.mod cmd/schick-order/
COPY cmd/schick-product/go.mod cmd/schick-product/
# Full source for what we build
COPY pkg/notification/ pkg/notification/
COPY cmd/schick-notification/ cmd/schick-notification/
RUN go build -o /schick-notification ./cmd/schick-notification

FROM alpine:3.21
RUN adduser -D -u 1000 app
USER app
COPY --from=builder /schick-notification /schick-notification
ENTRYPOINT ["/schick-notification"]
