FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o notifyq .

# ---- Runtime image ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /app
COPY --from=builder /app/notifyq .

EXPOSE 8080

ENV LISTEN_ADDR=:8080
ENV DATABASE_PATH=/data/notifications.db

VOLUME ["/data"]

CMD ["./notifyq"]
