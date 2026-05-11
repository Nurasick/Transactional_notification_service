FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/worker  ./cmd/worker
RUN go build -o /out/producer ./cmd/producer

# ---- worker image ----
FROM alpine:3.19 AS worker
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/worker /worker
ENTRYPOINT ["/worker"]

# ---- producer image ----
FROM alpine:3.19 AS producer
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/producer /producer
EXPOSE 8080
ENTRYPOINT ["/producer"]
