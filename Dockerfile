FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/osto ./cmd/osto

FROM alpine:3.21
RUN addgroup -S app && adduser -S app -G app
COPY --from=build /out/osto /usr/local/bin/osto
USER app
ENTRYPOINT ["/usr/local/bin/osto"]
