<?php

namespace App\Console\Commands;

use App\Models\Venda;
use Illuminate\Console\Command;

class ListarVendas extends Command
{
    protected $signature = 'vendas:listar';

    protected $description = 'Mostra as vendas e se já foram pagas';

    public function handle(): int
    {
        $this->table(
            ['id', 'cliente', 'valor', 'vencimento', 'status', 'título', 'paga em'],
            Venda::query()->orderBy('created_at')->get()->map(fn (Venda $v) => [
                $v->id,
                $v->cliente_id,
                number_format($v->valor_centavos / 100, 2, ',', '.'),
                $v->vencimento->format('Y-m-d'),
                $v->status,
                $v->titulo_id ?? '',
                $v->paga_em?->format('Y-m-d H:i:s') ?? '',
            ]),
        );

        return self::SUCCESS;
    }
}
