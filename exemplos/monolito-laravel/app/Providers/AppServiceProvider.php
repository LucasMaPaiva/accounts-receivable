<?php

namespace App\Providers;

use App\Mensageria\ConexaoRabbit;
use App\Mensageria\Publicador;
use App\Mensageria\RabbitPublicador;
use Illuminate\Support\ServiceProvider;

class AppServiceProvider extends ServiceProvider
{
    /**
     * Register any application services.
     */
    public function register(): void
    {
        $this->app->singleton(ConexaoRabbit::class, fn () => new ConexaoRabbit(config('services.rabbitmq.url')));
        // Singleton: o queue:work reaproveita a mesma conexão entre jobs.
        $this->app->singleton(Publicador::class, RabbitPublicador::class);
    }

    /**
     * Bootstrap any application services.
     */
    public function boot(): void
    {
        //
    }
}
