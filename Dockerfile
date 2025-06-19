FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o tuf-server server.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/tuf-server .
COPY --from=builder /app/tuf-repository ./tuf-repository
EXPOSE 8080
CMD ["./tuf-server", "--port", "8080"]