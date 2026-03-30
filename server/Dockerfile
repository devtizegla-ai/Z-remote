FROM golang:1.23-alpine AS build
WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o remote-server ./cmd/server

FROM alpine:3.20
WORKDIR /app
RUN addgroup -S app && adduser -S app -G app

COPY --from=build /app/remote-server /app/remote-server
COPY --from=build /app/migrations /app/migrations

RUN mkdir -p /app/data/uploads && chown -R app:app /app
USER app

EXPOSE 8080
ENV SERVER_ADDR=:8080
CMD ["/app/remote-server"]
