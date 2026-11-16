# Etapa 1: compila os binários estáticos (api, worker e simular-venda)
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

# Etapa 2: imagem final só com os binários (sem shell, sem compilador)
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
