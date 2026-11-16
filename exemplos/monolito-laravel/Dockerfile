# Etapa 1: dependências PHP sem as de desenvolvimento
FROM composer:2 AS vendor
WORKDIR /app
COPY composer.json composer.lock ./
# ext-sockets fica na imagem final; aqui só baixa os pacotes.
RUN composer install --no-dev --no-interaction --no-scripts --no-autoloader --prefer-dist --ignore-platform-req=ext-sockets
COPY . .
RUN composer dump-autoload --optimize --no-dev --no-scripts

# Etapa 2: PHP de linha de comando com a extensão sockets (php-amqplib)
FROM php:8.4-cli-alpine
RUN apk add --no-cache --virtual .deps $PHPIZE_DEPS linux-headers \
 && docker-php-ext-install sockets \
 && apk del .deps
WORKDIR /app
COPY --from=vendor /app /app
RUN php artisan package:discover \
 && mkdir -p database/dados storage/framework/cache storage/framework/views storage/logs \
 && chown -R www-data:www-data database/dados storage bootstrap/cache
USER www-data
CMD ["php", "artisan", "list"]
