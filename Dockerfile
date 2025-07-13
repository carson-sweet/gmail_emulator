# Build transformer
FROM golang:1.21-alpine AS transformer
WORKDIR /build
COPY transformer/enron_transformer.go .
RUN go build -o transformer enron_transformer.go

# Build emulator
FROM golang:1.21-alpine AS emulator
WORKDIR /build
COPY emulator/main.go .
RUN go mod init emulator && \
    go get github.com/gorilla/mux && \
    go get github.com/rs/cors && \
    go build -o gmail-emulator main.go

# Transform data (requires maildir in build context)
FROM alpine:latest AS data-transform
RUN apk add --no-cache ca-certificates
WORKDIR /transform
COPY --from=transformer /build/transformer .
COPY data/maildir /input
RUN ./transformer \
    --enron-path /input \
    --output /data \
    --user kaminski-v \
    --limit 5000 \
    --test-email test@example.com

# Final image with pre-transformed data
FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app

# Copy emulator binary
COPY --from=emulator /build/gmail-emulator .

# Copy pre-transformed data
COPY --from=data-transform /data /data

EXPOSE 8080
CMD ["./gmail-emulator", "--data", "/data", "--port", "8080"]