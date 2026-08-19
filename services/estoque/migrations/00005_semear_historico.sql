-- +goose Up

-- +goose StatementBegin
DO $$
DECLARE
    perfis CONSTANT jsonb := '[
        {"codigo": "PRD-001", "base": 1.0,  "tendencia": 1.0, "chance": 0.85},
        {"codigo": "PRD-002", "base": 1.4,  "tendencia": 2.6, "chance": 0.90},
        {"codigo": "PRD-003", "base": 7.5,  "tendencia": 1.1, "chance": 0.95},
        {"codigo": "PRD-004", "base": 0.35, "tendencia": 1.0, "chance": 0.20},
        {"codigo": "PRD-005", "base": 0.8,  "tendencia": 1.9, "chance": 0.60}
    ]'::jsonb;

    perfil       jsonb;
    produto      RECORD;
    dia          integer;
    progresso    numeric;
    quantidade   integer;
    consumo_total integer;
    saldo_corrente integer;
    momento      timestamptz;
    consumos     integer[];
    anomalia_dia integer;
BEGIN
    PERFORM setseed(0.42);

    FOR perfil IN SELECT * FROM jsonb_array_elements(perfis)
    LOOP
        SELECT id, codigo, saldo INTO produto
          FROM produtos
         WHERE codigo = perfil->>'codigo';

        CONTINUE WHEN NOT FOUND;

        consumos := ARRAY[]::integer[];
        consumo_total := 0;

        anomalia_dia := CASE WHEN (perfil->>'base')::numeric >= 1.0
                             THEN 20 + floor(random() * 40)::integer
                             ELSE -1 END;

        FOR dia IN 0..89 LOOP
            progresso := dia::numeric / 89.0;

            IF random() > (perfil->>'chance')::numeric THEN
                quantidade := 0;
            ELSE
                quantidade := greatest(1, round(
                    (perfil->>'base')::numeric
                    * (1 + ((perfil->>'tendencia')::numeric - 1) * progresso)
                    * (0.65 + random() * 0.7)
                )::integer);
            END IF;

            IF dia = anomalia_dia THEN
                quantidade := greatest(quantidade, 1) * 11;
            END IF;

            consumos := consumos || quantidade;
            consumo_total := consumo_total + quantidade;
        END LOOP;

        saldo_corrente := produto.saldo + consumo_total;
        momento := now() - interval '90 days';

        INSERT INTO movimentos_estoque
            (produto_id, tipo, quantidade, saldo_anterior, saldo_posterior, referencia, criado_em)
        VALUES
            (produto.id, 'AJUSTE', saldo_corrente, 0, saldo_corrente,
             'entrada inicial de estoque', momento);

        FOR dia IN 0..89 LOOP
            quantidade := consumos[dia + 1];
            CONTINUE WHEN quantidade = 0;

            momento := now()
                     - make_interval(days => 89 - dia)
                     + make_interval(hours => 8, mins => floor(random() * 540)::integer);

            INSERT INTO movimentos_estoque
                (produto_id, tipo, quantidade, saldo_anterior, saldo_posterior, referencia, criado_em)
            VALUES
                (produto.id, 'BAIXA', quantidade,
                 saldo_corrente, saldo_corrente - quantidade,
                 'NF-H' || lpad((dia + 1)::text, 5, '0'), momento);

            saldo_corrente := saldo_corrente - quantidade;
        END LOOP;

        IF saldo_corrente <> produto.saldo THEN
            RAISE EXCEPTION 'historico de % fecha em % mas o saldo e %',
                produto.codigo, saldo_corrente, produto.saldo;
        END IF;
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
DELETE FROM movimentos_estoque
 WHERE referencia = 'entrada inicial de estoque'
    OR referencia LIKE 'NF-H%';
