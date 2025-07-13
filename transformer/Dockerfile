FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY enron_transformer.go .
RUN go build -o transformer enron_transformer.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/transformer .

ENTRYPOINT ["./transformer"]