FROM golang:1.22.3-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app

COPY go.mod ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o file-uploader

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

RUN adduser -D -g '' appuser
USER appuser

WORKDIR /app

COPY --from=builder --chown=appuser /app/file-uploader ./

COPY --from=builder --chown=appuser /app/templates ./templates/

RUN mkdir -p uploads && chown appuser:appuser uploads

EXPOSE 8080

ENTRYPOINT ["./file-uploader"]