FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY main.go .
RUN go mod init emulator && \
    go get github.com/gorilla/mux && \
    go get github.com/rs/cors && \
    go build -o gmail-emulator main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/gmail-emulator .

EXPOSE 8080
CMD ["./gmail-emulator", "--data", "/data", "--port", "8080"]