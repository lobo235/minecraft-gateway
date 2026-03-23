FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o minecraft-gateway ./cmd/server

# ----

FROM alpine:3.21

RUN apk add --no-cache ca-certificates zstd

WORKDIR /app
COPY --from=builder /build/minecraft-gateway .

EXPOSE 8080

ENTRYPOINT ["/app/minecraft-gateway"]
