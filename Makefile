# Sem Go instalado, tudo roda no container oficial.
GO = docker run --rm -v $(CURDIR):/app -w /app -v gomod:/go/pkg/mod golang:1.23 go
MIGRATE = docker compose run --rm migrate -path /migrations -database "postgres://contas:contas@db:5432/contas?sslmode=disable"

.PHONY: test vet fmt up down reset migrate-up migrate-down migration psql logs

test:
	$(GO) test ./... -race -cover

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
