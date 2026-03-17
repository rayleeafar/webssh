FROM golang:1.25-alpine AS builder

# gcc and musl-dev are required for CGO; sqlite-dev provides the SQLite headers
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o server ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /app

COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]
