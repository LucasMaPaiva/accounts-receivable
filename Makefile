# Sem Go instalado, tudo roda no container oficial.
GO = docker run --rm -v $(CURDIR):/app -w /app -v gomod:/go/pkg/mod golang:1.23 go

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
