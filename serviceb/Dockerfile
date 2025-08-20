FROM golang:1.21-alpine as builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o serviceb .

FROM alpine:3.18

WORKDIR /app
COPY --from=builder /app/serviceb .

EXPOSE 8081

CMD ["./serviceb"]