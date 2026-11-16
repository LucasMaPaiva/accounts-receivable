<?php

namespace App\Mensageria;

/** Publica uma mensagem e só retorna depois que o broker confirmou. */
interface Publicador
{
    public function publicar(string $exchange, string $chave, string $messageId, array $corpo): void;
}
