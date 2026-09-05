FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /ordenes-api .

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /ordenes-api .

EXPOSE 8081

CMD ["./ordenes-api"]
