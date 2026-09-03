# Стадия сборки: полный Go-тулчейн, нужен только здесь.
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Сначала копируем только go.mod/go.sum и качаем зависимости так Docker
# закэширует этот слой и не будет заново скачивать зависимости при каждой
# правке кода, только когда меняется сам go.mod/go.sum.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 статическая линковка, бинарь не зависит от системных
# .so библиотек, поэтому спокойно запускается в минимальном alpine-образе.
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/server ./cmd/server

# Финальная стадия: только бинарь, без компилятора и исходников
# меньше размер образа, меньше поверхность атаки.
FROM alpine:3.20

WORKDIR /app
COPY --from=builder /build/server .

EXPOSE 8080

CMD ["./server"]