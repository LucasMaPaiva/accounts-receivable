<?php

namespace App\Console\Commands;

use App\Mensageria\ConexaoRabbit;
use App\Mensageria\MensagemInvalida;
use App\Mensageria\ProcessarTituloPago;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\Log;
use PhpAmqpLib\Message\AMQPMessage;
use Throwable;

/**
 * Processo longo que consome a fila monolito.titulo_pago (declarada pelo
 * serviço Go). Se cair, as mensagens sem ack voltam para a fila; o Docker
 * reinicia o processo.
 *
 *   - tratada ou repetida: ack
 *   - inválida: nack sem requeue (descartada; reprocessar não adianta)
 *   - erro temporário (banco travado, por exemplo): espera e nack com requeue
 */
class ConsumirTitulosPagos extends Command
{
    protected $signature = 'contas:consumir-titulos-pagos';

    protected $description = 'Dá baixa nas vendas quando o serviço de contas avisa que o título foi pago';

    public const FILA = 'monolito.titulo_pago';

    public function handle(ConexaoRabbit $rabbit, ProcessarTituloPago $processar): int
    {
        $conexao = $rabbit->abrir();
        $canal = $rabbit->canal($conexao);
        // Mesma declaração do worker Go (idempotente): não importa quem sobe primeiro.
        $canal->queue_declare(self::FILA, passive: false, durable: true, exclusive: false, auto_delete: false);
        $canal->basic_qos(0, 10, false); // no máximo 10 mensagens sem ack por vez

        $canal->basic_consume(self::FILA, '', false, false, false, false, function (AMQPMessage $msg) use ($processar) {
            $messageId = $msg->has('message_id') ? (string) $msg->get('message_id') : '';
            try {
                $resultado = $processar->executar($messageId, $msg->getBody());
                Log::info('titulo.pago tratado', ['message_id' => $messageId, 'resultado' => $resultado]);
                $msg->ack();
            } catch (MensagemInvalida $e) {
                Log::warning('titulo.pago descartado', ['message_id' => $messageId, 'erro' => $e->getMessage()]);
                $msg->nack(requeue: false);
            } catch (Throwable $e) {
                Log::error('erro temporário, titulo.pago volta para a fila', ['message_id' => $messageId, 'erro' => $e->getMessage()]);
                sleep(1);
                $msg->nack(requeue: true);
            }
        });

        Log::info('consumindo', ['fila' => self::FILA]);
        $canal->consume();

        return self::SUCCESS;
    }
}
