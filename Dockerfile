ARG SERVICE
FROM golang:1.26.3-alpine AS builder

ARG SERVICE
WORKDIR /workspace/${SERVICE}

# Local replace: github.com/elug3/dupli1/shared => ../shared
COPY shared/go.mod /workspace/shared/go.mod
COPY shared/ /workspace/shared/

COPY ${SERVICE}/go.mod ${SERVICE}/go.sum* ./

RUN go mod download

COPY ${SERVICE}/ .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /service ./cmd/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /service /usr/local/bin/service
ENTRYPOINT ["/usr/local/bin/service"]
