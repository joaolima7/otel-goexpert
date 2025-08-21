FROM golang:1.24-alpine as builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o servicea .

FROM alpine:3.18

WORKDIR /app
COPY --from=builder /app/servicea .

EXPOSE 8080

CMD ["./servicea"]