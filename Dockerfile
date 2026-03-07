# Stage 1: build
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 go build -o /harmonclaw ./cmd/harmonclaw/

# Stage 2: run
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /harmonclaw /harmonclaw
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/v1/health 2>/dev/null || exit 1
ENTRYPOINT ["/harmonclaw"]
