# Ultra-minimal scratch container for Beginning Android Relay
FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
# COPY go.sum ./
RUN go mod download || true

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o relay-server ./cmd/relay-server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/relay-server .

EXPOSE 8080
ENTRYPOINT ["./relay-server"]
