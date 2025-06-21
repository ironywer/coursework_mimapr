# --- Этап 1: билд Go-приложения с CGO на образе с glibc ---
FROM golang:1.24 AS builder
WORKDIR /app

# Устанавливаем gcc и либу SQLite для glibc
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      build-essential \
      libsqlite3-dev && \
    rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Включаем CGO и билдим
ENV CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64
ARG BUILD_SERVICE=cmd/p2p_node
RUN go build -trimpath -ldflags="-s -w" -o /go/bin/app ./${BUILD_SERVICE}

# --- Этап 2: финальный образ с Python (ваш существующий) ---
FROM python:3.8-slim
WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY style_transfer.py .
COPY --from=builder /go/bin/app /usr/local/bin/app

RUN mkdir -p received_images processed_images received_styles
ENTRYPOINT ["app"]
