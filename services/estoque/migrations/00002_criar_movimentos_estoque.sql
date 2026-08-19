-- +goose Up

CREATE TABLE movimentos_estoque (
    id              BIGSERIAL   PRIMARY KEY,
    produto_id      UUID        NOT NULL REFERENCES produtos(id),
    tipo            TEXT        NOT NULL,
    quantidade      INTEGER     NOT NULL,
    saldo_anterior  INTEGER     NOT NULL,
    saldo_posterior INTEGER     NOT NULL,

    referencia      TEXT,

    chave_idem      TEXT,

    criado_em       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT movimentos_tipo_valido  CHECK (tipo IN ('BAIXA', 'ESTORNO', 'AJUSTE')),
    CONSTRAINT movimentos_qtd_positiva CHECK (quantidade > 0)
);

CREATE INDEX movimentos_chave_idem ON movimentos_estoque (chave_idem) WHERE chave_idem IS NOT NULL;
CREATE INDEX movimentos_produto    ON movimentos_estoque (produto_id, criado_em DESC);

CREATE UNIQUE INDEX movimentos_estorno_unico
    ON movimentos_estoque (chave_idem, produto_id)
    WHERE tipo = 'ESTORNO';

-- +goose Down
DROP TABLE movimentos_estoque;
