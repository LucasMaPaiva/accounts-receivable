# Etapa 1: compila um binário estático
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /api ./cmd/api

# Etapa 2: imagem final só com o binário (sem shell, sem compilador)
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
