<?php

namespace App\Jobs;

use App\Mensageria\Publicador;
use App\Models\Venda;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Queue\Queueable;

/**
 * Avisa o serviço de contas a receber que uma venda foi faturada.
 *
 * Roda na fila "database" do próprio Laravel: o job é gravado na mesma
 * transação da venda (outbox sem código extra). Se o RabbitMQ estiver fora,
 * o job falha e o Laravel tenta de novo com espera crescente.
 */
class PublicarVendaFaturada implements ShouldQueue
{
    use Queueable;

    public int $tries = 10;

    public function __construct(public readonly string $vendaId) {}

    /** Segundos de espera entre as tentativas (a última se repete). */
    public function backoff(): array
    {
        return [1, 5, 15, 30, 60];
    }

    public function handle(Publicador $publicador): void
    {
        $venda = Venda::findOrFail($this->vendaId);

        $publicador->publicar('vendas', 'venda.faturada', $venda->eventoFaturadaId(), [
            'evento_id' => $venda->eventoFaturadaId(),
            'venda_id' => $venda->id,
            'cliente_id' => $venda->cliente_id,
            'descricao' => $venda->descricao,
            'valor_centavos' => $venda->valor_centavos,
            'vencimento' => $venda->vencimento->format('Y-m-d'),
        ]);
    }
}
