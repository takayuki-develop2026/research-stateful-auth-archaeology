FROM php:8.4-fpm

# =========================
# System packages & PHP extensions
# =========================
RUN apt-get update \
  && apt-get install -y \
     git \
     default-mysql-client \
     zlib1g-dev \
     libzip-dev \
     libpng-dev \
     libjpeg-dev \
     libwebp-dev \
     libpq-dev \
     unzip \
  && docker-php-ext-configure gd --with-jpeg --with-webp \
  && docker-php-ext-install -j$(nproc) \
     gd \
     pdo_mysql \
     pdo_pgsql \
     pgsql \
     zip \
     bcmath \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

# =========================
# Composer
# =========================
COPY --from=composer:2 /usr/bin/composer /usr/local/bin/composer

# =========================
# PHP-FPM config
# =========================
COPY ./conf.d/mailhog.ini /usr/local/etc/php/conf.d/mailhog.ini
COPY ./www.conf /usr/local/etc/php-fpm.d/www.conf

# =========================
# Workdir (Laravel root)
# =========================
WORKDIR /var/www/backend