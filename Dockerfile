# --- Build stage ---
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o server ./cmd/server

# --- Run stage ---
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/server .

# Templates and static files are read from disk at runtime (no build step).
COPY web/ web/

EXPOSE 8080

CMD ["./server"]
