<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Concerns\HasUlids;
use Illuminate\Database\Eloquent\Model;

class Venda extends Model
{
    use HasUlids;

    public const FATURADA = 'faturada';
    public const PAGA = 'paga';

    protected $fillable = ['cliente_id', 'descricao', 'valor_centavos', 'vencimento', 'status', 'titulo_id', 'paga_em'];

    protected $attributes = ['status' => self::FATURADA];

    protected function casts(): array
    {
        return [
            'valor_centavos' => 'integer',
            'vencimento' => 'date',
            'paga_em' => 'datetime',
        ];
    }

    /** Id estável do evento: se o job rodar de novo, o serviço Go reconhece a repetição. */
    public function eventoFaturadaId(): string
    {
        return 'venda.faturada:'.$this->id;
    }
}
