<?php

namespace App\Mensageria;

use PhpAmqpLib\Channel\AMQPChannel;
use PhpAmqpLib\Connection\AMQPStreamConnection;
use PhpAmqpLib\Message\AMQPMessage;
use RuntimeException;
use Throwable;

/**
 * Publica com publisher confirms: só considera enviado quando o RabbitMQ
 * confirma. Com "mandatory", uma mensagem que não chega a nenhuma fila volta
 * e vira erro, em vez de sumir em silêncio.
 *
 * A conexão fica aberta entre um job e outro (o queue:work é um processo
 * longo); se der erro, ela é descartada e o próximo job abre outra.
 */
final class RabbitPublicador implements Publicador
{
    private ?AMQPStreamConnection $conexao = null;

    private ?AMQPChannel $canal = null;

    public function __construct(private readonly ConexaoRabbit $rabbit) {}

    public function publicar(string $exchange, string $chave, string $messageId, array $corpo): void
    {
        $recusada = null;
        try {
            $canal = $this->canal();
            $canal->set_nack_handler(function () use (&$recusada) {
                $recusada = 'o RabbitMQ recusou a mensagem (nack)';
            });
            $canal->set_return_listener(function ($codigo, $texto) use (&$recusada) {
                $recusada = "mensagem sem fila de destino: {$texto}";
            });

            $canal->basic_publish(new AMQPMessage(json_encode($corpo, JSON_THROW_ON_ERROR), [
                'content_type' => 'application/json',
                'delivery_mode' => AMQPMessage::DELIVERY_MODE_PERSISTENT,
                'message_id' => $messageId,
                'timestamp' => time(),
            ]), $exchange, $chave, mandatory: true);

            $canal->wait_for_pending_acks_returns(5);
        } catch (Throwable $e) {
            $this->fechar();
            throw $e;
        }

        if ($recusada !== null) {
            throw new RuntimeException($recusada);
        }
    }

    private function canal(): AMQPChannel
    {
        if ($this->canal === null || ! $this->canal->is_open()) {
            $this->conexao = $this->rabbit->abrir();
            $this->canal = $this->rabbit->canal($this->conexao);
            $this->canal->confirm_select();
        }

        return $this->canal;
    }

    private function fechar(): void
    {
        try {
            $this->conexao?->close();
        } catch (Throwable) {
            // conexão já estava quebrada
        }
        $this->conexao = $this->canal = null;
    }
}
