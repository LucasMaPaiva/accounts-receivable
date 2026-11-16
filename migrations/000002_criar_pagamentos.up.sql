-- Cada pagamento vira uma linha: o histórico fica guardado, não só o total.
CREATE TABLE pagamentos (
    id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    titulo_id      text        NOT NULL REFERENCES titulos (id),
    valor_centavos bigint      NOT NULL CHECK (valor_centavos > 0),
    pago_em        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX pagamentos_titulo_idx ON pagamentos (titulo_id);
