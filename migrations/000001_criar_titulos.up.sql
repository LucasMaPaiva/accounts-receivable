-- Títulos a receber. Dinheiro em centavos (bigint), nunca numeric/float na aplicação.
-- As regras críticas também ficam no banco: se a aplicação errar, o CHECK barra.
CREATE TABLE titulos (
    id             text        PRIMARY KEY,
    cliente_id     text        NOT NULL CHECK (cliente_id <> ''),
    descricao      text        NOT NULL DEFAULT '',
    valor_centavos bigint      NOT NULL CHECK (valor_centavos > 0),
    pago_centavos  bigint      NOT NULL DEFAULT 0 CHECK (pago_centavos >= 0),
    vencimento     date        NOT NULL,
    status         text        NOT NULL DEFAULT 'aberto',
    criado_em      timestamptz NOT NULL DEFAULT now(),
    atualizado_em  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT titulos_pago_nao_excede CHECK (pago_centavos <= valor_centavos),
    CONSTRAINT titulos_status_coerente CHECK (status = CASE
        WHEN pago_centavos = 0              THEN 'aberto'
        WHEN pago_centavos = valor_centavos THEN 'pago'
        ELSE 'parcial'
    END)
);

-- Listagem por cliente filtrando status e ordenando por vencimento.
CREATE INDEX titulos_cliente_status_venc_idx ON titulos (cliente_id, status, vencimento);

-- Títulos vencidos: só interessam os que não estão pagos (índice parcial, menor).
CREATE INDEX titulos_em_aberto_venc_idx ON titulos (vencimento) WHERE status <> 'pago';
