FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /fleet-dispatch ./cmd/api

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /fleet-dispatch /app/fleet-dispatch
EXPOSE 8080
CMD ["/app/fleet-dispatch"]
