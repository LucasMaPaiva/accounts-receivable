<?php

namespace App\Mensageria;

use RuntimeException;

/** Erro permanente: reprocessar não adianta, a mensagem é descartada da fila. */
final class MensagemInvalida extends RuntimeException {}
