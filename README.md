# accounts-receivable

Microsserviço de contas a receber em Go, pensado como a extração de um módulo de um monólito Laravel.

O monólito (PHP) continua responsável pelo cadastro e pelas telas. Este serviço cuida dos títulos a receber: emissão,
pagamento parcial ou total, saldo e vencimento. A comunicação com o monólito será assíncrona, por fila.

## Por que Go aqui

- Processo único e de longa duração: atende muitas requisições e consome fila ao mesmo tempo (goroutines), com pouca
  memória.
- Binário estático: a imagem Docker fica pequena e sobe em milissegundos.
- Tipagem estática e erros como valor de retorno: as regras de dinheiro ficam explícitas.

## Estrutura

```
cmd/api/              ponto de entrada (servidor HTTP com desligamento gracioso)
migrations/           SQL versionado (golang-migrate), aplicado por um container antes da API subir
internal/titulo/      domínio: regras do título, sem HTTP nem banco
internal/repositorio/ onde os títulos ficam (hoje em memória)
internal/api/         handlers HTTP (net/http da biblioteca padrão)
```

Dinheiro sempre em centavos (`int64`), nunca `float`.

## Rodar

Sem Go instalado, pelo Docker:

```sh
make test   # testes com detector de condição de corrida
make up     # Postgres, migrations e API em http://localhost:8080 (imagem distroless)
make down   # para tudo (make reset apaga também o banco)
make psql   # console do banco (Postgres em 127.0.0.1:5444)
make migration nome=criar_algo   # nova migration
make migrate-down / make migrate-up
```

```sh
curl -X POST localhost:8080/titulos \
  -d '{"cliente_id":"c1","descricao":"Mensalidade","valor_centavos":10000,"vencimento":"2026-12-10"}'
curl -X POST localhost:8080/titulos/<id>/pagamentos -d '{"valor_centavos":4000}'
curl localhost:8080/titulos/<id>
```

## Roteiro

- [x] Fase 1: domínio com testes e API HTTP com repositório em memória
- [ ] Fase 2: PostgreSQL
  - [x] docker-compose com Postgres 17, healthcheck e migrations antes da API
  - [x] tabelas `titulos` e `pagamentos` com as regras também no banco (`CHECK` de valor, pago não excede o valor,
        status coerente com o pago; chave estrangeira no pagamento) e índices para listagem e vencidos
  - [ ] repositório com pgx implementando a mesma interface do repositório em memória
  - [ ] pagamento numa transação com `SELECT ... FOR UPDATE` e teste de pagamentos simultâneos
  - [ ] listagem com filtros (cliente, status, vencidos)
- [ ] Fase 3: fila (RabbitMQ). O Laravel publica `venda.faturada`; um worker em Go consome, cria o título e publica
      `titulo.pago`. Idempotência pelo id da mensagem e reprocessamento com fila de erro
- [ ] Fase 4: lado Laravel de exemplo (job que publica o evento e listener que recebe o pagamento)
- [ ] Fase 5: servidor MCP só leitura para consultar saldo, títulos vencidos e inadimplência por cliente
- [ ] Fase 6: observabilidade (logs estruturados com slog, métricas Prometheus) e CI no GitHub Actions
