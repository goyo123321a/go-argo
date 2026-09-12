# --- 构建阶段 ---
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o app

# --- 运行阶段 ---
FROM alpine:latest

RUN apk add --no-cache \
    openssl \
    curl \
    bash \
    wget \
    gcompat \
    iproute2 \
    coreutils \
    ca-certificates

RUN addgroup -g 1001 -S nodejs && adduser -S nodejs -u 1001 -G nodejs

WORKDIR /tmp
COPY --from=builder /app/app /usr/local/bin/app
COPY index.html /tmp/
RUN chmod +x /usr/local/bin/app

USER nodejs

ENV PORT=3000
EXPOSE 3000
CMD ["/usr/local/bin/app"]
