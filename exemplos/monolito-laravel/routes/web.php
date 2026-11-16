<?php

use Illuminate\Support\Facades\Route;

// O exemplo é só de linha de comando (artisan); a rota existe para o healthcheck.
Route::get('/', fn () => ['app' => 'monolito-laravel', 'ok' => true]);
