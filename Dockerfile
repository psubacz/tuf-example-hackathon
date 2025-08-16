FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o tuf-server ./cmd/tuf-server

FROM alpine:latest
RUN apk --no-cache add ca-certificates && \
    adduser -D -s /bin/sh -u 1000 appuser
WORKDIR /app
COPY --from=builder /app/tuf-server ./tuf-server
RUN chmod +x ./tuf-server && chown 1000:1000 ./tuf-server
COPY --from=builder --chown=1000:1000 /app/tuf-repository-v2 ./tuf-repository
USER 1000
EXPOSE 8080
CMD ["./tuf-server", "--port", "8080", "--repo", "/app/tuf-repository"]