-- Inbox: cada evento recebido é registrado na mesma transação que cria o
-- título. Se a mesma mensagem chegar de novo (a fila entrega "pelo menos uma
-- vez"), o INSERT não insere nada e o título não é duplicado.
CREATE TABLE eventos_processados (
    evento_id     text        PRIMARY KEY,
    processado_em timestamptz NOT NULL DEFAULT now()
);

-- Outbox: eventos a publicar gravados na mesma transação da mudança.
-- Um relay lê os pendentes e publica no RabbitMQ; se a fila estiver fora do
-- ar, o evento espera aqui em vez de se perder.
CREATE TABLE outbox (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tipo         text        NOT NULL,
    payload      jsonb       NOT NULL,
    criado_em    timestamptz NOT NULL DEFAULT now(),
    publicado_em timestamptz
);

CREATE INDEX outbox_pendentes_idx ON outbox (id) WHERE publicado_em IS NULL;
