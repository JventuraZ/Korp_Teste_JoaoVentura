-- +goose Up

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE produtos (
    id            UUID        PRIMARY KEY,
    codigo        TEXT        NOT NULL,
    descricao     TEXT        NOT NULL,

    saldo         INTEGER     NOT NULL DEFAULT 0 CHECK (saldo >= 0),

    versao        BIGINT      NOT NULL DEFAULT 1,

    ativo         BOOLEAN     NOT NULL DEFAULT TRUE,
    criado_em     TIMESTAMPTZ NOT NULL DEFAULT now(),
    atualizado_em TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT produtos_codigo_unico UNIQUE (codigo),
    CONSTRAINT produtos_codigo_nao_vazio    CHECK (length(btrim(codigo))    BETWEEN 1 AND 50),
    CONSTRAINT produtos_descricao_nao_vazia CHECK (length(btrim(descricao)) BETWEEN 1 AND 200)
);

CREATE INDEX produtos_codigo_trgm    ON produtos USING gin (codigo    gin_trgm_ops);
CREATE INDEX produtos_descricao_trgm ON produtos USING gin (descricao gin_trgm_ops);

-- +goose Down
DROP TABLE produtos;
