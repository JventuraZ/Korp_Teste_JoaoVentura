-- +goose Up

INSERT INTO produtos (id, codigo, descricao, saldo) VALUES
    (gen_random_uuid(), 'PRD-001', 'Parafuso sextavado M6 x 40mm', 10),
    (gen_random_uuid(), 'PRD-002', 'Porca sextavada M6 zincada',    4),
    (gen_random_uuid(), 'PRD-003', 'Arruela lisa M6 inox',        250),
    (gen_random_uuid(), 'PRD-004', 'Rolamento rigido de esferas 6204', 1),
    (gen_random_uuid(), 'PRD-005', 'Correia dentada HTD 5M',        7)
ON CONFLICT (codigo) DO NOTHING;

-- +goose Down
DELETE FROM produtos WHERE codigo IN ('PRD-001','PRD-002','PRD-003','PRD-004','PRD-005');
