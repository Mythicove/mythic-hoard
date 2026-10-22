# ---- build ----
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /hoard ./cmd/server

# ---- run ----
FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /hoard /usr/local/bin/hoard
EXPOSE 8081
ENTRYPOINT ["hoard"]
