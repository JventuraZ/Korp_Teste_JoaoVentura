FROM docker.io/library/golang:1.26-alpine AS construcao

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /app/fiscal ./cmd/fiscal

FROM docker.io/library/alpine:3.21

RUN adduser -D -u 10001 korp

COPY --from=construcao /app/fiscal /app/fiscal

USER korp
EXPOSE 8083

ENTRYPOINT ["/app/fiscal"]
