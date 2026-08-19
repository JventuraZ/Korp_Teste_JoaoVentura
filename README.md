# Korp — Faturamento com Estoque

Teste técnico: cadastro de produtos, cadastro de notas fiscais e impressão de nota com baixa de
estoque, construído como **microsserviços** em linguagens diferentes, com um frontend Angular.

| Camada | Tecnologia | Porta |
|---|---|---|
| Frontend | Angular 22 (standalone, zoneless, signals) + PrimeNG 22, servido por nginx | 8080 |
| Serviço de Estoque | Go 1.26 · chi · pgx · goose | 8081 |
| Serviço de Faturamento | .NET 10 · Minimal API · EF Core · Polly | 8082 |
| Serviço Fiscal | Go 1.26 · chi · motor de regras dirigido a dados | 8083 |
| Banco | PostgreSQL 18, um database por serviço | 5432 |

O detalhamento técnico pedido no enunciado está em
**[docs/detalhamento-tecnico.md](docs/detalhamento-tecnico.md)**; o contrato do Estoque, em
**[docs/contrato-estoque.md](docs/contrato-estoque.md)**.

---

## Pré-requisitos

### Caminho A — containers (recomendado)

Só precisa de **Docker** ou **Podman**, com Compose. Cada serviço builda sua própria imagem com o
toolchain (Go, .NET, Node) já embutido — nada mais para instalar.

| Ferramenta | Onde pegar |
|---|---|
| Docker Desktop | [docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop/) |
| Podman + podman-compose | [podman.io](https://podman.io/docs/installation) |

Instalação por linha de comando, se preferir:

```powershell
# Windows (winget)
winget install -e --id Docker.DockerDesktop
```

```bash
# macOS (Homebrew)
brew install --cask docker

# Linux (Debian/Ubuntu) — Docker Engine + plugin compose
curl -fsSL https://get.docker.com | sh
sudo apt install docker-compose-plugin
```

Depois de instalado, pule para [Como rodar](#como-rodar).

> 🪟 **Windows:** o Docker Desktop usa o WSL2 como motor. Se a instalação avisar que o WSL2 não está
> habilitado, abra um **PowerShell como Administrador** e rode `wsl --install`, reinicie a máquina e
> só então abra o Docker Desktop pela primeira vez.

### Caminho B — nativo, sem container

Só necessário para rodar/depurar um serviço fora do Docker (ex.: com o debugger da IDE). O Postgres
continua melhor em container mesmo assim.

| Ferramenta | Versão usada neste projeto | Necessária para |
|---|---|---|
| [Git](https://git-scm.com/) | 2.55 | clonar o repositório |
| [Go](https://go.dev/dl/) | 1.26 (o `go.mod` fixa `1.26.6`; o próprio `go` baixa esse patch automaticamente na primeira build) | Estoque, Fiscal |
| [.NET SDK](https://dotnet.microsoft.com/download) | 10.0 | Faturamento |
| [Node.js](https://nodejs.org/) | 22 LTS ou superior | Frontend |
| Docker ou Podman | qualquer | só para o container do Postgres |

```powershell
# Windows (winget) — instala tudo de uma vez
winget install -e --id Git.Git
winget install -e --id GoLang.Go
winget install -e --id Microsoft.DotNet.SDK.10
winget install -e --id OpenJS.NodeJS.LTS
winget install -e --id Docker.DockerDesktop
```

```bash
# macOS (Homebrew)
brew install git go node dotnet-sdk
brew install --cask docker

# Linux (Debian/Ubuntu)
sudo apt install git nodejs npm
# Go: https://go.dev/doc/install (o pacote do apt costuma estar desatualizado)
# .NET SDK: https://learn.microsoft.com/dotnet/core/install/linux
```

Depois de instalar as ferramentas, abra um terminal **novo** (para carregar o PATH) e baixe as
dependências de cada serviço — um comando por linguagem, cada um lendo o arquivo de lockfile do
próprio serviço (`go.sum`, `package-lock.json`, os `.csproj`), sem tocar em nada fora da sua pasta:

```bash
cd services/estoque      && go mod download
cd ../fiscal              && go mod download
cd ../faturamento         && dotnet restore Faturamento.slnx
cd ../../frontend         && npm install
```

Confirme que tudo ficou disponível:

```bash
git --version && go version && dotnet --version && node --version && npm --version
```

---

## Como rodar

```bash
cp .env.example .env
podman-compose up -d --build      # ou: docker compose up -d --build
```

**Aplicação em http://localhost:8080.** Sobem cinco containers: Postgres, Estoque, Faturamento,
Fiscal e o frontend (build de produção servido por nginx, que também roteia `/api` para o
microsserviço dono de cada rota — o navegador enxerga uma origem só e não há CORS no caminho). As
migrations são aplicadas pelos próprios serviços na subida e o Estoque semeia cinco produtos: não há
passo manual de schema.

Para iterar no frontend com recarga automática, deixe os serviços no compose e rode
`cd frontend && npm install && npm start` (porta 4200) — o `proxy.conf.json` aponta para as mesmas
portas, então as duas formas convivem.

<details>
<summary>Rodar os serviços nativos, sem container</summary>

Requer as ferramentas do **Caminho B** acima, com as dependências já baixadas (`go mod download`,
`dotnet restore`, `npm install`). O Postgres continua em container:

```bash
docker run -d --name korp-postgres -e POSTGRES_USER=korp -e POSTGRES_PASSWORD=korp_dev_senha \
  -e POSTGRES_DB=postgres -p 127.0.0.1:5432:5432 \
  -v "$(pwd)/infra/postgres/init:/docker-entrypoint-initdb.d:ro" postgres:18-alpine
```

```bash
# Estoque — aplica as migrations e semeia produtos sozinho na subida
cd services/estoque && go build -o estoque ./cmd/estoque
env ESTOQUE_PORT=8081 \
    ESTOQUE_DATABASE_URL='postgres://korp:korp_dev_senha@localhost:5432/db_estoque?sslmode=disable' \
    ./estoque

# Fiscal — protótipo sem banco, sobe isolado
cd services/fiscal && go build -o fiscal ./cmd/fiscal
env FISCAL_PORT=8083 ./fiscal

# Faturamento
cd services/faturamento
env ASPNETCORE_URLS='http://+:8082' \
    ConnectionStrings__Postgres='Host=localhost;Port=5432;Database=db_faturamento;Username=korp;Password=korp_dev_senha' \
    dotnet run --project Faturamento.Api --no-launch-profile

# Frontend, com recarga automática
cd frontend && npm start      # http://localhost:4200, proxy.conf.json aponta para as portas acima
```

Estoque e Faturamento **se recusam a subir** sem a string de conexão, em vez de cair num padrão
embutido: credencial não fica em arquivo versionado.
</details>

---

## O fluxo principal

O exemplo literal do enunciado: produto com saldo 10, nota usando 2 unidades, saldo final 8.

1. **Notas fiscais → Nova nota.** A numeração sequencial (`NF-000001`) vem de uma sequence do
   Postgres, não de um `max(numero) + 1` no código — que duplicaria sob concorrência.
2. Inclua produtos pelo autocomplete. **Incluir item não reserva estoque**: o débito acontece uma
   única vez, na impressão.
3. **Imprimir** → indicador de processamento → status **Fechada** e saldo debitado. Nota Fechada não
   pode ser impressa de novo.
4. **Previsão** mostra quanto tempo o saldo de cada produto ainda dura; o contador de itens críticos
   aparece no cabeçalho sem precisar abrir a tela.

---

## Requisitos obrigatórios

### 1. Arquitetura de microsserviços

Serviços em linguagens diferentes, com **database-per-service**: não existe FK nem JOIN atravessando
a fronteira. O Faturamento guarda o código do produto e um **snapshot da descrição**; o saldo pertence
ao Estoque e só é obtido por HTTP. O contrato do Estoque foi escrito **antes** da implementação, e o
Faturamento foi construído contra o documento — não contra o código Go.

### 2. Tratamento de falhas

```bash
./scripts/demo-falha.sh       # ciclo completo, verificado no terminal
```

Ou à mão, para acompanhar **pela tela**:

```bash
podman stop korp-estoque      # derruba o serviço
podman ps                     # confere quem está de pé
podman start korp-estoque     # religa
```

Com a aplicação aberta, observe nesta ordem:

1. o **selo do cabeçalho vira vermelho sozinho** em até 5s — é sondagem, o usuário não clicou em nada;
2. **Notas continua carregando**; só Produtos e Previsão dependem do Estoque;
3. **imprimir** devolve `503` com mensagem acionável, depois de 3 tentativas com backoff exponencial
   e jitter (Polly) — e a nota **permanece Aberta**, sem nenhum efeito colateral;
4. religado o serviço, a **mesma nota** imprime normalmente.

O Faturamento responde o tempo todo: o `depends_on` do compose deliberadamente não exige o Estoque
saudável, e o `/health` devolve `200` com `"estoque":"indisponivel"` — este serviço está saudável,
quem não está é a dependência. O disjuntor abre após 4 falhas e passa a falhar rápido, em vez de
empilhar chamadas num serviço que já caiu.

> ⏱️ **Espere ~10s antes de reimprimir.** O disjuntor fica aberto por `BreakDuration = 10s`
> ([Program.cs](services/faturamento/Faturamento.Api/Program.cs)). Tentar logo após o `podman start`
> ainda falha — e a recuperação parece não ter funcionado quando está fazendo exatamente o que deveria.

**O que cada serviço derruba** — medido, não estimado:

| Serviço parado | Produtos | Notas | Fiscal | Aplicação | Impressão |
|---|---|---|---|---|---|
| *nenhum* | 200 | 200 | 200 | 200 | ✅ |
| `korp-estoque` | **502** | 200 | 200 | 200 | **503**, nota segue Aberta |
| `korp-faturamento` | 200 | **502** | 200 | 200 | — |
| `korp-fiscal` | 200 | 200 | **502** | 200 | ✅ |
| `korp-postgres` | **500** | **500** | 200 | 200 | — |

Duas leituras: **a falha não se propaga** — cada serviço cai sozinho e a aplicação continua sendo
servida em todos os casos, inclusive com o banco fora. E **`502` significa outra coisa que `500`**:
`502` é o nginx não encontrando o serviço, o processo morreu; `500` é o serviço no ar, respondendo,
mas sem conseguir falar com o banco. Quem for diagnosticar em produção precisa dessa distinção.

> ⚠️ Nos serviços nativos, derrube pelo PID da porta:
> `kill $(ss -lntpH | grep ':8081' | grep -oP 'pid=\K[0-9]+' | head -1)`. Não use
> `pkill -f 'Faturamento.Api'` — o padrão casa com a própria linha de comando e o shell se mata.
> Use `pkill -f 'Faturamento[.]Api'`.

### 3. Persistência real

PostgreSQL 18. `db_estoque` (produtos, movimentos_estoque, idempotencia) e `db_faturamento`
(notas_fiscais, itens_nota). As migrations viajam dentro dos binários: goose com `embed.FS` no Go,
EF Core Migrations no C#.

---

## Requisitos opcionais entregues

### (a) Concorrência — `./scripts/demo-concorrencia.sh`

Seis notas disputando a **última unidade** de um produto: exatamente uma é impressa, cinco recebem
`422`, o saldo termina em zero e nunca fica negativo. Duas camadas independentes garantem isso:

- `SELECT ... ORDER BY codigo FOR UPDATE` serializa as transações. **A ordem estável é obrigatória**:
  sem ela, duas notas com os mesmos produtos em ordem inversa entram em deadlock.
- `CHECK (saldo >= 0)` na tabela recusaria a escrita se a lógica da aplicação falhasse.

A mesma nota impressa em duas janelas ao mesmo tempo é barrada por concorrência otimista, usando a
coluna de sistema `xmin` do Postgres como token — sem coluna extra nem trigger.

### (c) Idempotência — `./scripts/demo-idempotencia.sh`

Repetir a mesma `Idempotency-Key` devolve a **resposta original gravada**, byte a byte, sem debitar
de novo — provado contando os registros da trilha de auditoria. É o que acontece quando a resposta se
perde no caminho de volta e o retry dispara: sem essa garantia, os dois requisitos obrigatórios
(retry + persistência) se combinariam num bug de estoque debitado em dobro.

- a chave é gravada na nota **antes** da chamada remota, então o retry reusa a mesma chave;
- a identidade da requisição é o **SHA-256 do corpo canônico** (itens agregados e ordenados) —
  reenviar o mesmo pedido com outra formatação é retry, não conflito;
- mesma chave com corpo **diferente** devolve `409`, não a resposta antiga: devolver a antiga seria
  mentir sobre uma operação que nunca foi executada;
- **editar os itens descarta a chave pendente** — sem isso, uma nota recusada por saldo ficaria
  travada para sempre, porque o usuário corrigiria a quantidade e receberia conflito de chave;
- o **estorno também é idempotente**, garantido por índice único, e não apaga a baixa original: grava
  um movimento `ESTORNO`, preservando a auditoria.

### (b) Uso de IA — previsão de ruptura e detecção de anomalias

Tela **Previsão** (`/insights`). Em vez de avisar que *"PRD-002 está baixo"*, o sistema responde
**"PRD-002 acaba em ~1 dia no ritmo atual"** — e essa diferença é o ponto. Um alerta por limite fixo
(`saldo <= 5`) não distingue um produto com 10 unidades que gira 5 por dia, e acaba depois de amanhã,
de outro com as mesmas 10 unidades que gira uma por mês. O que importa é a **taxa**.

- **Previsão:** média móvel exponencial (EWMA, α = 0.10, meia-vida ≈ 6,6 dias) sobre o consumo
  líquido diário. O peso maior no passado recente faz um produto cujo consumo dobrou nesta semana
  subir para crítico, em vez de ter a mudança diluída em três meses de calmaria.
- **Anomalias:** escore-z modificado de Iglewicz-Hoaglin, com **mediana e MAD** em vez de média e
  desvio-padrão — estes dois são deslocados justamente pelos outliers que se quer encontrar.

Três detalhes decidem se o resultado é confiável ou lixo:

- **Dias sem movimento entram como zero.** Somar só os dias com venda faria um produto que vendeu uma
  vez em 90 dias parecer vender todo dia.
- **Estorno abate consumo**, senão o sistema pediria reposição de mercadoria que nunca saiu.
- **Materialidade separada de significância estatística.** Num item cuja baixa típica é 1 unidade,
  uma baixa de 2 é estatisticamente extrema e praticamente irrelevante. Sem esse filtro o painel
  abria com 15 alertas, 12 deles ruído — foi um bug real, encontrado com dados reais.

Roda **offline**: sem chave de API, sem serviço externo, sem custo por chamada. O cálculo vive em
[`internal/analise`](services/estoque/internal/analise/analise.go) como funções puras, testado sem
banco. A migration [`00005`](services/estoque/migrations/00005_semear_historico.sql) semeia 90 dias de
histórico, então a tela nasce populada — inclusive na máquina de quem for avaliar.

---

## A saga de impressão

Dois bancos separados significam que não existe transação distribuída. A ordem dos passos é
deliberada:

```
0. grava a chave de idempotência na nota          (local, antes de tudo)
1. POST /api/estoque/baixas                       (remoto, idempotente)
2. fecha a nota                                   (local)
3. se o passo 2 falhar → POST .../estorno         (compensação)
```

Inverter 1 e 2 criaria nota fechada sem baixa — e nada a compensar do outro lado. O passo 3 cobre o
único caminho que a saga não consegue evitar: o Estoque commitou e o fechamento falhou depois.

---

## Mapeamento de erros

Os serviços respondem erro em **RFC 7807** (`application/problem+json`), o que permite ao Angular ter
**um único interceptor** para backends em linguagens diferentes. Ele decide pelo campo `type`, não
pelo status: dois erros podem compartilhar o `409` e pedir ações opostas do usuário.

| Situação | Estoque | Faturamento | Por quê |
|---|---|---|---|
| Saldo insuficiente | `409` | **`422`** | Para o Estoque é conflito de estado do **produto**. Para o Faturamento a **nota está válida**, apenas não é processável agora |
| Nota já fechada | — | `409` | Conflito real de estado da **nota** |
| Estoque fora do ar | — | `503` | Retries esgotados ou disjuntor aberto |

Na tela isso vira mensagens diferentes: `422` = "falta estoque, ajuste as quantidades";
`409` = "esta nota já foi impressa, recarregue".

---

## API

**Estoque (8081)** — os 9 endpoints e seus contratos estão em
[docs/contrato-estoque.md](docs/contrato-estoque.md). Os dois que importam para a saga:
`POST /api/estoque/baixas` (baixa atômica, exige header `Idempotency-Key`) e
`POST /api/estoque/baixas/{chave}/estorno` (compensação).

**Faturamento (8082)**

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/api/notas` · `/api/notas/{id}` | Lista paginada e nota com itens |
| `POST` | `/api/notas` | Cria nota Aberta com numeração sequencial |
| `POST` · `DELETE` | `/api/notas/{id}/itens` | Inclui item (valida no Estoque e grava snapshot da descrição) e remove |
| `POST` | `/api/notas/{id}/impressao` | **Dispara a saga** |
| `GET` | `/health` · `/health/vivo` | `200` mesmo com o Estoque caído; a sonda de vida não consulta a dependência |

---

## Testes

```bash
cd services/estoque
env TEST_DATABASE_URL='postgres://korp:korp_dev_senha@localhost:5432/db_estoque_teste?sslmode=disable' \
    go test ./...

cd services/faturamento
env 'TEST_FATURAMENTO_CONEXAO=Host=localhost;Port=5432;Database=db_faturamento_teste;Username=korp;Password=korp_dev_senha' \
    dotnet test
```

64 testes, cobrindo validação, idempotência, estorno, concorrência, a saga e sua compensação. Os de
integração rodam contra Postgres de verdade, porque as garantias que eles verificam (`FOR UPDATE`,
`CHECK`, índice único de estorno, sequence, `xmin`) são do banco — um dublê em memória passaria sem
provar nada. Sem as variáveis de ambiente eles **se auto-pulam**, para a suíte continuar verde numa
máquina sem banco. Os bancos `db_estoque_teste` e `db_faturamento_teste` precisam existir.

---

## Aba Fiscal / Tributação

Terceiro microsserviço (`services/fiscal/`), em **Produtos → abrir um produto → aba Fiscal /
Tributação**. Cobre classificação fiscal, PIS/COFINS, IPI, regras tributárias por operação, validação
e simulação.

> ⚠️ **Protótipo com dados fictícios.** Alíquotas, CFOPs e NCMs são exemplos para demonstrar a
> interface e **não foram conferidos contra a legislação**. Nada é persistido em banco.

**A decisão que sustenta tudo: regra tributária é dado, não `if`.** A legislação brasileira muda com
frequência, e um sistema que codifica alíquota em `switch` precisa de deploy a cada alteração. Aqui a
regra é um registro que um motor genérico interpreta — condições (operação, UF origem/destino, tipo
de cliente, consumidor final) resultando em CFOP, CST/CSOSN e alíquotas. **Campo nulo é curinga**, e
daí saem três comportamentos, sem nenhum conhecimento fiscal embutido no código:

- **Especificidade** — quantas condições a regra fixa. A regra que descreve melhor a operação vence,
  e a prioridade só desempata. Ninguém precisa ordenar regras à mão.
- **Conflito** — duas regras ativas, de mesma especificidade, com condições sobrepostas. Se a
  prioridade também empata, o resultado dependeria da ordem de leitura: é erro. Se difere, é aviso.
- **Trilha de explicação** — o *"Como chegamos a este cálculo?"* é **tipo de retorno** do motor, não
  log. Requisito de produto que depende de vasculhar log não sobrevive ao primeiro refactor.

A fronteira entre o real e o simulado é deliberada. Casamento de regras, detecção de conflitos e
validação **estrutural** (NCM ausente, regra sem CFOP, CST sem alíquota) são lógica determinística,
independente de legislação, e estão implementados de verdade — com **17 testes** em
[`internal/regras/`](services/fiscal/internal/regras/), rodando sem banco e sem HTTP. Já os valores
monetários são simulados e rotulados na tela: *"esta alíquota está certa para este NCM"* exige a
legislação, e qualquer resposta seria invenção. Por isso a simulação declara na resposta o que não
considera (DIFAL, MVA, exclusões de base, regimes especiais).

---

## Limitações conhecidas

Lacunas conscientes, não esquecimentos.

| Limitação | Situação |
|---|---|
| **Sem autenticação** | Fora do escopo do enunciado. Por isso só a porta `8080` é publicada; banco e serviços internos ficam em `127.0.0.1` |
| **Sem correlação entre serviços** | O `RequestID` do chi não atravessa a fronteira HTTP; faltaria propagar `traceparent` (W3C Trace Context) |
| **`idempotencia` cresce sem limite** | Falta expurgo por idade; o índice `idempotencia_criado_em` já existe para isso |
| **Análise sem `LIMIT`** | `PreverRupturas` e `Anomalias` carregam a janela inteira em memória — suficiente para uma demonstração, não para um ERP |
| **`amostras` superestima a confiança** | `PreencherDias` completa a janela toda, então um produto novo reporta 91 amostras e `SEM_DADOS` nunca dispara |
| **Compensação sem outbox** | Se o estorno falhar há `LogCritical` com a chave, mas a recuperação é manual. O estorno é idempotente, então uma fila com retry resolveria |
| **Sem limites de recurso nos containers** | Um serviço com vazamento pode consumir a máquina inteira |
| **Sem cabeçalhos de segurança no nginx** | Faltam CSP, `X-Content-Type-Options` e afins |
| **Sem testes no frontend** | Os backends têm 64 testes; o Angular não |
| **Aba fiscal sem persistência** | A configuração vive na sessão do navegador. O contrato HTTP já é o definitivo — quando houver banco, muda o repositório, não a API |
| **Tributos simplificados** | Aritmética sobre as alíquotas cadastradas; sem DIFAL, MVA nem exclusões de base |
| **Seed fabrica auditoria** | A `00005` cria 90 dias de movimentos que nunca aconteceram. Aceitável para demonstrar a previsão, inaceitável num sistema real |

---

## Estrutura

```
docs/                       contrato do Estoque, detalhamento técnico, roteiro do vídeo
infra/postgres/init/        criação dos dois databases
scripts/                    demonstrações dos requisitos (roteiro do vídeo)
services/estoque/           Go — cmd, internal/{dominio,repositorio,transporte,analise}, migrations
services/faturamento/       C# — Dominio, Dados, Estoque, Servicos, Web + testes
frontend/                   Angular — nucleo (serviços e interceptor), produtos, notas, insights
```
