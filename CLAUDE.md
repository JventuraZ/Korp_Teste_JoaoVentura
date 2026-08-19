# Korp_Teste_JoaoVentura — contexto do projeto

Teste técnico da Korp: sistema de faturamento com estoque, em **microsserviços**.
Entrega = repositório público no GitHub + **vídeo** de apresentação + **detalhamento técnico**,
enviados para rh@korp.com.br. Prazo acordado: **2026-08-20** (3 dias a partir de 2026-08-17).

## O que o enunciado exige

**Funcionalidades:** cadastro de produtos (código, descrição, saldo); cadastro de notas fiscais
(numeração sequencial, status Aberta/Fechada, múltiplos produtos com quantidades); impressão da
nota (indicador de processamento → status vira Fechada → saldo debitado; só notas Abertas).

**Obrigatórios:** (1) ≥2 microsserviços; (2) cenário de falha de um serviço com recuperação e
feedback ao usuário; (3) persistência real em banco.

**Opcionais:** (a) concorrência, (b) IA e (c) idempotência — **os três entregues**.

**O detalhamento técnico precisa responder, item a item:** ciclos de vida do Angular usados; se e
como RxJS foi usado; outras bibliotecas e finalidade; bibliotecas de componentes visuais;
gerenciamento de dependências no Go; frameworks Go/C#; tratamento de erros e exceções no backend;
se usou LINQ e de que forma.

## Arquitetura

```
Angular 22 + PrimeNG 22  (frontend/, :8080 via nginx no compose,
                          ou :4200 com `npm start` em desenvolvimento)
        │  nginx (produção) ou proxy.conf.json (dev) roteiam por rota
        ├── /api/produtos, /api/estoque  →  Estoque (Go)       :8081
        ├── /api/notas                   →  Faturamento (C#)   :8082
        └── /api/fiscal                  →  Fiscal (Go)        :8083
                                                │
                          ambos → Postgres 18 :5432, database-per-service
                                  (db_estoque, db_faturamento)
```

**Database-per-service é real:** não há FK nem JOIN atravessando a fronteira. O Faturamento
guarda o código do produto e um **snapshot da descrição**; o saldo pertence ao Estoque e só é
consultado por HTTP.

### Serviço de Estoque (Go)

`services/estoque/` — chi/v5, pgx/v5 (SQL escrito à mão, sem ORM), goose com `embed.FS`
(migrations aplicadas na subida do próprio binário), Go Modules.

`internal/analise/` é a parte preditiva (requisito opcional b): **funções puras**, sem banco nem
HTTP. EWMA (α=0.10) para estimar consumo diário e escore-z modificado com mediana/MAD para
anomalias. Roda offline, sem chave de API. A migration `00005` semeia 90 dias de histórico —
sem ela o painel abre vazio, porque não há de onde prever.

O contrato HTTP está em [docs/contrato-estoque.md](docs/contrato-estoque.md) e foi escrito
**antes** da implementação. **É a fonte da verdade** — mudou o comportamento, atualize o documento
junto. Um script confere os 9 endpoints contra ele.

O núcleo é `internal/repositorio/baixas.go`:
- `AplicarBaixa` — uma transação: `SELECT ... ORDER BY codigo FOR UPDATE` (**a ordem estável é
  obrigatória**; sem ela, duas notas com os mesmos produtos em ordem inversa dão deadlock), valida
  **todos** os saldos antes de qualquer `UPDATE` para poder listar todos os faltantes de uma vez,
  grava os movimentos e persiste a resposta na tabela `idempotencia` **na mesma transação**.
- `Estornar` — grava movimentos `ESTORNO` sem apagar a baixa original.
- Idempotência por SHA-256 do corpo **canônico** (itens agregados e ordenados), então reenviar o
  mesmo pedido com outra formatação é retry, não conflito.

`internal/transporte/erros.go::MapearErro` é o **ponto único** que traduz erro de domínio para
`application/problem+json`. Handlers nunca escolhem status.

### Serviço de Faturamento (C#)

`services/faturamento/` — Minimal API, EF Core 10 + Npgsql, Polly v8 via
`Microsoft.Extensions.Http.Resilience`. LINQ usado de forma visível nas projeções.

`Servicos/ServicoImpressao.cs` é a **saga**, e a ordem dos passos é deliberada:
0. grava a chave de idempotência **antes** de qualquer chamada externa;
1. debita o Estoque (passo remoto, seguro para repetir por causa da chave);
2. fecha a nota localmente;
3. se o passo 2 falhar, **estorna** o passo 1.

Inverter 1 e 2 criaria nota fechada sem baixa — e nada a compensar do outro lado.

`Web/ManipuladorDeErros.cs` é o equivalente ao `MapearErro` do Go: traduz exceção para RFC 7807.

### Serviço Fiscal (Go)

`services/fiscal/` — aba **Fiscal / Tributação** do cadastro de produtos. **Protótipo sem banco**:
tabelas de referência e exemplos vêm de JSON embutido com `embed.FS`, e os dados fiscais são
**fictícios**, rotulados como tal na tela.

O núcleo é `internal/regras/`: regra tributária é **dado**, não `if`. Condição nula é curinga, e daí
saem a **especificidade** (a regra que descreve melhor a operação vence; prioridade só desempata), a
**detecção de conflitos** (mesma especificidade + sobreposição; ambíguo se a prioridade também
empata) e a **trilha de explicação**, que é tipo de retorno do motor e não log.

A fronteira é deliberada: casamento e conflito são determinísticos e implementados de verdade;
cálculo monetário é simulado e declara o que não considera (DIFAL, MVA, exclusões de base).

### Frontend

`frontend/` — Angular 22 standalone, **zoneless**, signals. PrimeNG 22 com tema Aura.
`nucleo/erro.interceptor.ts` é um **interceptor único** que atende os dois microsserviços — só é
possível porque Go e C# respondem erro no mesmo formato RFC 7807. Ele decide pelo campo `type`,
não pelo status, porque dois erros podem compartilhar o 409 e pedir ações opostas do usuário.

## Decisões que parecem estranhas e não são

- **`409` no Estoque vira `422` no Faturamento** para saldo insuficiente. Para o Estoque é
  conflito de estado do *produto*; para o Faturamento a *nota está válida*, apenas não é
  processável agora. Na tela, `409` = "recarregue" e `422` = "ajuste as quantidades".
- **Mesma chave de idempotência com corpo diferente devolve 409**, não a resposta antiga —
  devolver a antiga seria mentir sobre uma operação que nunca foi executada.
- **Editar os itens de uma nota descarta a chave pendente** (`ServicoNotas.InvalidarChave`). Sem
  isso, uma nota recusada por saldo ficaria travada para sempre: o usuário corrige a quantidade,
  manda imprimir, e o Estoque recusa por conflito de chave.
- **`Id` das entidades EF não tem inicializador.** Com a chave preenchida antes do `SaveChanges`,
  o rastreador de grafo classifica a entidade como existente e emite `UPDATE` em vez de `INSERT`.
  Isso já causou um bug real aqui.
- **O Faturamento não depende do Estoque no `depends_on`** do compose: precisa subir e responder
  com o Estoque fora do ar. É o requisito obrigatório 2.
- **O healthcheck do Faturamento devolve 200 mesmo com o Estoque caído** (com
  `"estoque":"indisponivel"` no corpo). O serviço está saudável; a dependência é que não está.
- **O nginx do frontend resolve DNS por variável** (`set $destino_estoque`, mais `resolver`). Com
  `proxy_pass` fixo ele cacheia o IP na subida e, depois de `podman stop/start korp-estoque`, o
  container volta com outro IP e a tela fica em 502 para sempre — justo depois do momento mais
  importante do vídeo. O endereço do resolvedor é detectado por `20-detectar-dns.sh`, porque muda
  entre podman e Docker.
- **A detecção de anomalias exige materialidade além de significância estatística.** Sem isso,
  num item cuja baixa típica é 1 unidade, toda baixa de 2 virava alerta.
- **A compensação da saga usa `CancellationToken` próprio**, nunca o da requisição. Herdar o token
  fazia o caso mais comum de falha virar o pior: usuário fecha a aba → token cancelado →
  `SaveChanges` estoura → a compensação seria abortada antes de sair, deixando estoque debitado
  numa nota aberta. Há teste de regressão que cancela no meio da saga.
- **`/health/vivo` é separado de `/health`.** O healthcheck do container não pode depender do
  Estoque: `/health` consulta a dependência via Polly e pode levar >15s, marcando o Faturamento
  unhealthy por causa do vizinho — o oposto do requisito 2.
- **Não há CORS em nenhum dos serviços, de propósito.** Nas duas topologias suportadas (nginx em
  produção, `proxy.conf.json` em dev) o navegador nunca faz requisição cross-origin.
- **CFOP e CST de ICMS não ficam no cadastro do produto.** Dependem da operação, origem, destino e
  cliente — por isso vivem nas regras. Fixá-los no produto seria o erro que o requisito de CFOPs
  múltiplos pede para evitar.
- **`FISCAL_PORT` precisa existir no `.env`.** É a única porta que caía no default `${...:-8083}`, e
  o podman-compose trata esse ramo como inteiro, quebrando a interpolação com `TypeError`.
- **Só a porta 8080 é publicada na rede.** Banco e serviços internos escutam em `127.0.0.1` —
  com `0.0.0.0` e a senha padrão do repositório, a LAN inteira lia e escrevia no banco (foi
  confirmado explorando).
- **O seed de histórico nunca toca `produtos.saldo`.** Ele constrói a narrativa para frente, a
  partir de um AJUSTE inicial, de modo a terminar exatamente no saldo que a 00004 definiu — os
  testes e as demos dependem de `PRD-001`=10 e `PRD-004`=1.

## Convenções

- **Tudo em português**: código, identificadores, comentários, documentação, mensagens.
- Comentários de código **sem acentos**; textos voltados ao usuário (JSON `title`/`detail`, UI)
  **com acentos**.
- Comentários explicam **por quê**, não o quê.

## Como rodar

Toolchains instalados em espaço de usuário (`~/.local/go`, `~/.local/node`, `~/.dotnet`), com
symlinks em `~/.local/bin`. `dotnet` exige `DOTNET_ROOT=$HOME/.dotnet` no ambiente.

```bash
# Banco
podman run -d --name korp-postgres -e POSTGRES_USER=korp -e POSTGRES_PASSWORD=korp_dev_senha \
  -e POSTGRES_DB=postgres -p 5432:5432 \
  -v $PWD/infra/postgres/init:/docker-entrypoint-initdb.d:ro,z docker.io/library/postgres:18-alpine

# Estoque (aplica as migrations e semeia produtos sozinho na subida)
cd services/estoque && go build -o estoque ./cmd/estoque
env ESTOQUE_PORT=8081 \
    ESTOQUE_DATABASE_URL='postgres://korp:korp_dev_senha@localhost:5432/db_estoque?sslmode=disable' \
    ./estoque

# Faturamento
cd services/faturamento
env ASPNETCORE_URLS='http://+:8082' dotnet run --project Faturamento.Api --no-launch-profile

# Frontend
cd frontend && npx ng serve     # http://localhost:4200
```

Alternativa em containers: `podman-compose up -d --build` (o frontend continua no host).

## Testes

```bash
cd services/estoque
env TEST_DATABASE_URL='postgres://korp:korp_dev_senha@localhost:5432/db_estoque_teste?sslmode=disable' \
    go test ./...

cd services/faturamento
env 'TEST_FATURAMENTO_CONEXAO=Host=localhost;Port=5432;Database=db_faturamento_teste;Username=korp;Password=korp_dev_senha' \
    dotnet test
```

Sem essas variáveis os testes de integração **se auto-pulam** de propósito, para a suíte continuar
verde numa máquina sem banco. Os bancos `db_estoque_teste` e `db_faturamento_teste` precisam
existir (`CREATE DATABASE`).

`go test -race` **não funciona nesta máquina**: exige cgo e não há gcc instalado.

## Scripts de demonstração

`scripts/demo-falha.sh`, `demo-idempotencia.sh`, `demo-concorrencia.sh` — são o roteiro do vídeo,
um por requisito. Aceitam `EST=` e `FAT=` por variável de ambiente. O `demo-falha.sh` usa
`podman stop korp-estoque`, então exige o Estoque **em container**.

## Armadilhas do ambiente

- `pkill -f 'Faturamento.Api'` mata o próprio shell que roda o comando (a linha de comando casa com
  o padrão). Use `pkill -f 'Faturamento[.]Api'`.
- Variáveis com ponto no nome (`Logging__LogLevel__Microsoft.EntityFrameworkCore...`) exigem
  `env 'NOME=valor' comando`; bash rejeita `NOME=valor comando`.
- `podman-compose` foi instalado via `python3 -m pip install --user` (não havia pip no sistema).

## Estado atual (2026-08-17)

**Pronto e verificado:**

- Estoque completo — 9 endpoints conferidos contra o contrato, 22 testes unitários + 11 de
  integração (idempotência, estorno, concorrência com 8 goroutines, previsão e anomalias).
- Faturamento completo — saga com compensação, Polly, 13 testes, imagem de 130 MB.
- Frontend — três telas (Produtos, Notas, Previsão), interceptor único de erro, indicador de
  impressão, selo de saúde e contador de itens críticos no cabeçalho.
- Três scripts de demo, todos executados com sucesso.
- `README.md`, `docs/detalhamento-tecnico.md` (responde as 8 perguntas item a item) e
  `docs/roteiro-video.md`.

- `podman-compose up -d --build` validado do zero: os três containers sobem juntos, healthchecks
  verdes, e os três scripts de demo passam contra a stack em containers.
- 8 commits em `main`, contando a construção camada por camada.

**Pendente:**

1. Criar o repositório público `Korp_Teste_JoaoVentura` no GitHub e dar push (o usuário faz).
2. Gravar o vídeo seguindo [docs/roteiro-video.md](docs/roteiro-video.md).
3. Enviar para rh@korp.com.br: link do repositório, link do vídeo e o detalhamento técnico.
