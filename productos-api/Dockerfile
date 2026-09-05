FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /productos-api .

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /productos-api .

EXPOSE 8080

CMD ["./productos-api"]
