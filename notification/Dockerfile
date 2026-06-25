FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
# Full source for what we build
COPY notification/ notification/
RUN go build -o /schick-notification ./notification/cmd

FROM alpine:3.21
RUN adduser -D -u 1000 app
USER app
COPY --from=builder /schick-notification /schick-notification
ENTRYPOINT ["/schick-notification"]
