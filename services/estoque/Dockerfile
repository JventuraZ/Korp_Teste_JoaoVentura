# Multi-stage: a imagem final nao carrega compilador, codigo-fonte nem gerenciador
# de pacotes -- so o binario estatico e os certificados.

FROM docker.io/library/golang:1.26-alpine AS construcao

WORKDIR /src

# go.mod e go.sum primeiro: enquanto as dependencias nao mudam, esta camada vem
# do cache e o build nao rebaixa a rede a cada alteracao de codigo.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO desligado produz binario estatico, requisito para rodar numa imagem sem libc.
# -s -w remove tabela de simbolos e DWARF (~30% menor, sem perda em producao).
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /app/estoque ./cmd/estoque

FROM docker.io/library/alpine:3.21

# Usuario sem privilegios: se o processo for comprometido, nao e root no container.
RUN adduser -D -u 10001 korp

COPY --from=construcao /app/estoque /app/estoque

USER korp
EXPOSE 8081

# O healthcheck do compose chama este mesmo binario com -healthcheck, o que
# dispensa instalar curl ou wget so para verificar a saude.
ENTRYPOINT ["/app/estoque"]
