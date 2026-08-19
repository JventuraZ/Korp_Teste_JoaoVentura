-- +goose Up

CREATE TABLE idempotencia (
    chave           TEXT        PRIMARY KEY,
    endpoint        TEXT        NOT NULL,

    hash_requisicao TEXT        NOT NULL,

    status_http     INTEGER     NOT NULL,
    corpo_resposta  JSONB       NOT NULL,

    criado_em       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idempotencia_criado_em ON idempotencia (criado_em);

-- +goose Down
DROP TABLE idempotencia;
