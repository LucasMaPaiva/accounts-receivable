<?php

namespace Tests\Feature;

use App\Jobs\PublicarVendaFaturada;
use App\Mensageria\Publicador;
use App\Models\Venda;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Illuminate\Support\Facades\Queue;
use Tests\TestCase;

class FaturarVendaTest extends TestCase
{
    use RefreshDatabase;

    public function test_fatura_a_venda_e_enfileira_o_evento(): void
    {
        Queue::fake();

        $this->artisan('vendas:faturar', ['cliente' => 'c1', 'valor_centavos' => 15000, '--vencimento' => '2026-12-10'])
            ->assertSuccessful();

        $venda = Venda::sole();
        $this->assertSame(Venda::FATURADA, $venda->status);
        Queue::assertPushed(PublicarVendaFaturada::class, fn ($job) => $job->vendaId === $venda->id);
    }

    public function test_recusa_valor_invalido_sem_gravar_nada(): void
    {
        Queue::fake();

        $this->artisan('vendas:faturar', ['cliente' => 'c1', 'valor_centavos' => 0])->assertFailed();

        $this->assertSame(0, Venda::count());
        Queue::assertNothingPushed();
    }

    public function test_job_publica_o_contrato_esperado_pelo_servico_go(): void
    {
        $venda = Venda::create(['cliente_id' => 'c1', 'valor_centavos' => 15000, 'vencimento' => '2026-12-10']);
        $publicador = new class implements Publicador
        {
            public array $publicadas = [];

            public function publicar(string $exchange, string $chave, string $messageId, array $corpo): void
            {
                $this->publicadas[] = compact('exchange', 'chave', 'messageId', 'corpo');
            }
        };

        $job = new PublicarVendaFaturada($venda->id);
        $job->handle($publicador);
        $job->handle($publicador); // nova tentativa do mesmo job

        [$primeira, $segunda] = $publicador->publicadas;
        $this->assertSame('vendas', $primeira['exchange']);
        $this->assertSame('venda.faturada', $primeira['chave']);
        $this->assertSame([
            'evento_id' => 'venda.faturada:'.$venda->id,
            'venda_id' => $venda->id,
            'cliente_id' => 'c1',
            'descricao' => '',
            'valor_centavos' => 15000,
            'vencimento' => '2026-12-10',
        ], $primeira['corpo']);
        // Mesmo evento_id na repetição: o serviço Go não cria título duplicado.
        $this->assertSame($primeira['corpo']['evento_id'], $segunda['corpo']['evento_id']);
    }
}
