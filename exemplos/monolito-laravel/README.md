# monolito-laravel (exemplo)

O lado PHP da integração com o `accounts-receivable`. Laravel 13, SQLite e php-amqplib.

| Peça | O que faz |
|---|---|
| `vendas:faturar {cliente} {valor_centavos}` | grava a venda e, na mesma transação, o job `PublicarVendaFaturada` |
| `App\Jobs\PublicarVendaFaturada` | publica `venda.faturada` com publisher confirms; 10 tentativas com espera crescente |
| `contas:consumir-titulos-pagos` | consome `monolito.titulo_pago` e dá baixa na venda |
| `App\Mensageria\ProcessarTituloPago` | regra da baixa, idempotente pelo `message_id` (testável sem fila) |
| `vendas:listar` | mostra as vendas e se já foram pagas |

Rodar a partir da raiz do repositório: `make monolito-up`, `make faturar args="c1 15000"`, `make vendas` e
`make monolito-test`.
