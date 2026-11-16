# Sem Go instalado, tudo roda no container oficial.
GO = docker run --rm -v $(CURDIR):/app -w /app -v gomod:/go/pkg/mod golang:1.23 go
MIGRATE = docker compose run --rm migrate -path /migrations -database "postgres://contas:contas@db:5432/contas?sslmode=disable"

.PHONY: test test-integracao vet fmt tidy up down reset migrate-up migrate-down migration psql logs venda

test:
	$(GO) test ./... -race -cover

# Sobe o Postgres, cria o banco contas_teste (separado do de desenvolvimento),
# aplica as migrations nele e roda os testes marcados com a tag integracao.
TEST_DB_URL = postgres://contas:contas@db:5432/contas_teste?sslmode=disable
test-integracao:
	docker compose up -d --wait db
	docker compose exec -T db sh -c 'createdb -U contas contas_teste 2>/dev/null || true'
	docker compose run --rm migrate -path /migrations -database "$(TEST_DB_URL)" up
	docker run --rm --network accounts-receivable_default \
		-e DATABASE_URL="$(TEST_DB_URL)" \
		-v $(CURDIR):/app -w /app -v gomod:/go/pkg/mod golang:1.23 \
		go test -tags integracao -race -count=1 ./...

tidy:
	$(GO) mod tidy

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

up:
	docker compose up --build -d

down:
	docker compose down

# Apaga também o volume do banco.
reset:
	docker compose down -v

migrate-up:
	$(MIGRATE) up

# Desfaz só a última migration.
migrate-down:
	$(MIGRATE) down 1

# Uso: make migration nome=criar_algo
migration:
	docker run --rm -u $$(id -u):$$(id -g) -v $(CURDIR)/migrations:/migrations migrate/migrate:v4.18.1 create -ext sql -dir /migrations -seq $(nome)

psql:
	docker compose exec db psql -U contas -d contas

logs:
	docker compose logs -f api

# Uso: make venda  ou  make venda args="-cliente c2 -valor 9900"
venda:
	docker compose run --rm simular-venda $(args)
