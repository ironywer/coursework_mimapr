# --- Этап 1: билд Go-приложения ---
FROM golang:1.24-alpine AS builder

# Отключаем CGO для более лёгких бинарников
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

# Загружаем модули
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь проект
COPY . .

# Флаг BUILD_SERVICE позволяет выбрать что собирать: server или p2p_node
ARG BUILD_SERVICE=cmd/p2p_node
RUN go build -trimpath -ldflags "-s -w" -o /go/bin/app ./${BUILD_SERVICE}

# --- Этап 2: финальный образ с Python ---
FROM python:3.8-slim

WORKDIR /app

# Копируем только нужные файлы
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY style_transfer.py .
# Можно копировать полностью директорию internal, если Python-скрипт там что-то импортирует:
# COPY internal/style/processor.go /not/used 
# (он не нужен внутри контейнера, просто пример)

# Копируем скомпилированный Go-бинарник
COPY --from=builder /go/bin/app /usr/local/bin/app

# Создаём рабочие директории для входных/выходных файлов
RUN mkdir -p received_images processed_images received_styles

# По умолчанию — запуск Go-приложения
ENTRYPOINT ["app"]
