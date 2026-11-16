<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::create('vendas', function (Blueprint $table) {
            $table->ulid('id')->primary();
            $table->string('cliente_id');
            $table->string('descricao')->default('');
            $table->unsignedBigInteger('valor_centavos'); // dinheiro em centavos, igual ao serviço Go
            $table->date('vencimento');
            $table->string('status')->default('faturada'); // faturada | paga
            $table->string('titulo_id')->nullable();        // preenchido quando o título é quitado
            $table->timestamp('paga_em')->nullable();
            $table->timestamps();
        });

        // Inbox: message_id de cada titulo.pago já tratado. A fila entrega
        // "pelo menos uma vez"; a chave primária impede tratar duas vezes.
        Schema::create('mensagens_recebidas', function (Blueprint $table) {
            $table->string('message_id')->primary();
            $table->timestamp('recebida_em')->useCurrent();
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('mensagens_recebidas');
        Schema::dropIfExists('vendas');
    }
};
