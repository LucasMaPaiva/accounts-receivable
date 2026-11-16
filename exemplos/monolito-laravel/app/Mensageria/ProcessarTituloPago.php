<?php

namespace App\Mensageria;

use App\Models\Venda;
use Illuminate\Support\Carbon;
use Illuminate\Support\Facades\DB;

/**
 * Dá baixa na venda quando o serviço de contas avisa que o título foi quitado.
 * Não depende do RabbitMQ, então dá para testar sem fila.
 */
final class ProcessarTituloPago
{
    public const BAIXADA = 'baixada';
    public const REPETIDA = 'repetida';
    public const SEM_VENDA = 'sem_venda';                     // título criado direto na API do serviço
    public const VENDA_DESCONHECIDA = 'venda_desconhecida';

    public function executar(string $messageId, string $corpo): string
    {
        if ($messageId === '') {
            throw new MensagemInvalida('mensagem sem message_id');
        }
        $dados = json_decode($corpo, true);
        if (! is_array($dados) || empty($dados['titulo_id'])) {
            throw new MensagemInvalida('JSON inválido ou sem titulo_id');
        }

        // Registrar a mensagem e dar baixa na mesma transação: ou as duas
        // coisas ficam, ou nenhuma (e a mensagem volta para a fila).
        return DB::transaction(function () use ($messageId, $dados) {
            $nova = DB::table('mensagens_recebidas')->insertOrIgnore([
                'message_id' => $messageId,
                'recebida_em' => now(),
            ]);
            if ($nova === 0) {
                return self::REPETIDA;
            }
            if (empty($dados['venda_id'])) {
                return self::SEM_VENDA;
            }

            $venda = Venda::query()->lockForUpdate()->find($dados['venda_id']);
            if ($venda === null) {
                return self::VENDA_DESCONHECIDA;
            }
            $venda->update([
                'status' => Venda::PAGA,
                'titulo_id' => $dados['titulo_id'],
                'paga_em' => isset($dados['pago_em']) ? Carbon::parse($dados['pago_em']) : now(),
            ]);

            return self::BAIXADA;
        });
    }
}
