<?php

namespace Tests\Feature;

use App\Mensageria\MensagemInvalida;
use App\Mensageria\ProcessarTituloPago;
use App\Models\Venda;
use Illuminate\Foundation\Testing\RefreshDatabase;
use Tests\TestCase;

class ProcessarTituloPagoTest extends TestCase
{
    use RefreshDatabase;

    private function evento(Venda $venda, array $extra = []): string
    {
        return json_encode([
            'titulo_id' => 't-1',
            'venda_id' => $venda->id,
            'cliente_id' => 'c1',
            'valor_centavos' => 15000,
            'pago_em' => '2026-11-20T13:00:00Z',
        ] + $extra);
    }

    public function test_da_baixa_na_venda(): void
    {
        $venda = Venda::create(['cliente_id' => 'c1', 'valor_centavos' => 15000, 'vencimento' => '2026-12-10']);

        $resultado = app(ProcessarTituloPago::class)->executar('outbox-1', $this->evento($venda));

        $this->assertSame(ProcessarTituloPago::BAIXADA, $resultado);
        $venda->refresh();
        $this->assertSame(Venda::PAGA, $venda->status);
        $this->assertSame('t-1', $venda->titulo_id);
        $this->assertSame('2026-11-20 13:00:00', $venda->paga_em->utc()->format('Y-m-d H:i:s'));
    }

    public function test_mensagem_repetida_e_ignorada(): void
    {
        $venda = Venda::create(['cliente_id' => 'c1', 'valor_centavos' => 15000, 'vencimento' => '2026-12-10']);
        $processar = app(ProcessarTituloPago::class);

        $processar->executar('outbox-1', $this->evento($venda));
        $resultado = $processar->executar('outbox-1', $this->evento($venda, ['titulo_id' => 't-outro']));

        $this->assertSame(ProcessarTituloPago::REPETIDA, $resultado);
        $this->assertSame('t-1', $venda->refresh()->titulo_id);
        $this->assertDatabaseCount('mensagens_recebidas', 1);
    }

    public function test_titulo_sem_venda_ou_venda_desconhecida_nao_quebra(): void
    {
        $processar = app(ProcessarTituloPago::class);

        $this->assertSame(ProcessarTituloPago::SEM_VENDA,
            $processar->executar('outbox-2', json_encode(['titulo_id' => 't-2'])));
        $this->assertSame(ProcessarTituloPago::VENDA_DESCONHECIDA,
            $processar->executar('outbox-3', json_encode(['titulo_id' => 't-3', 'venda_id' => 'nao-existe'])));
    }

    public function test_mensagem_invalida_e_erro_permanente(): void
    {
        foreach (['JSON quebrado' => ['outbox-4', '{'], 'sem titulo_id' => ['outbox-5', '{}'], 'sem message_id' => ['', '{"titulo_id":"t"}']] as $caso => [$id, $corpo]) {
            try {
                app(ProcessarTituloPago::class)->executar($id, $corpo);
                $this->fail("{$caso}: deveria lançar MensagemInvalida");
            } catch (MensagemInvalida) {
                $this->addToAssertionCount(1);
            }
        }
        $this->assertDatabaseCount('mensagens_recebidas', 0);
    }
}
