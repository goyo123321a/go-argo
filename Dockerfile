# 多阶段构建
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

WORKDIR /app

ENV CGO_ENABLED=0
ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum* ./
RUN if [ -f go.mod ]; then go mod download; fi

COPY . .

ARG TARGETOS
ARG TARGETARCH

RUN go build -ldflags="-w -s" -o myapp .

FROM alpine:3.18

RUN apk add --no-cache ca-certificates curl && \
    update-ca-certificates

RUN addgroup -g 1000 -S appuser && \
    adduser -u 1000 -S appuser -G appuser && \
    mkdir -p /app/.tmp

COPY --from=builder --chown=appuser:appuser /app/myapp /app/
COPY --chown=appuser:appuser index.html /app/ 2>/dev/null || true

USER appuser

WORKDIR /app

EXPOSE 7860

HEALTHCHECK --interval=30s --timeout=3s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:7860/ || exit 1

CMD ["/app/myapp"]
