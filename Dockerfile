FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o app

FROM alpine:latest
RUN apk add --no-cache openssl curl bash wget gcompat iproute2 coreutils
RUN addgroup -g 1001 -S nodejs && adduser -S nodejs -u 1001 -G nodejs
WORKDIR /tmp
COPY --from=builder /app/app /usr/local/bin/app
COPY index.html /tmp/
RUN chmod +x /usr/local/bin/app
USER nodejs
EXPOSE 7860
CMD ["/usr/local/bin/app"]
