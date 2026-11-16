<?php

namespace App\Console\Commands;

use App\Jobs\PublicarVendaFaturada;
use App\Models\Venda;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Validator;

class FaturarVenda extends Command
{
    protected $signature = 'vendas:faturar
        {cliente : id do cliente}
        {valor_centavos : valor em centavos}
        {--vencimento= : AAAA-MM-DD (padrão: daqui a 30 dias)}
        {--descricao= : descrição da venda}';

    protected $description = 'Fatura uma venda e avisa o serviço de contas a receber';

    public function handle(): int
    {
        $dados = [
            'cliente_id' => $this->argument('cliente'),
            'valor_centavos' => $this->argument('valor_centavos'),
            'vencimento' => $this->option('vencimento') ?? now()->addDays(30)->format('Y-m-d'),
            'descricao' => $this->option('descricao') ?? '',
        ];
        $validador = Validator::make($dados, [
            'cliente_id' => ['required', 'string'],
            'valor_centavos' => ['required', 'integer', 'min:1'],
            'vencimento' => ['required', 'date_format:Y-m-d'],
        ]);
        if ($validador->fails()) {
            foreach ($validador->errors()->all() as $erro) {
                $this->error($erro);
            }

            return self::FAILURE;
        }

        // Venda e job na mesma transação (a fila é a tabela jobs do mesmo banco).
        $venda = DB::transaction(function () use ($dados) {
            $venda = Venda::create($dados);
            PublicarVendaFaturada::dispatch($venda->id);

            return $venda;
        });

        $this->info("venda {$venda->id} faturada; evento na fila para o serviço de contas");

        return self::SUCCESS;
    }
}
