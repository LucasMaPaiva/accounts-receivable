<?php

namespace App\Mensageria;

use PhpAmqpLib\Channel\AMQPChannel;
use PhpAmqpLib\Connection\AMQPStreamConnection;

/** Abre a conexão a partir de uma URL amqp://usuario:senha@host:porta/vhost. */
final class ConexaoRabbit
{
    public function __construct(private readonly string $url) {}

    public function abrir(): AMQPStreamConnection
    {
        $p = parse_url($this->url);
        $vhost = isset($p['path']) && $p['path'] !== '/' ? rawurldecode(substr($p['path'], 1)) : '/';

        return new AMQPStreamConnection(
            $p['host'] ?? 'localhost',
            $p['port'] ?? 5672,
            rawurldecode($p['user'] ?? 'guest'),
            rawurldecode($p['pass'] ?? 'guest'),
            $vhost,
            read_write_timeout: 60.0, // precisa ser pelo menos 2x o heartbeat
            heartbeat: 30,
        );
    }

    public function canal(AMQPStreamConnection $conexao): AMQPChannel
    {
        return $conexao->channel();
    }
}
