#!/bin/sh
set -eu

DESTINO=/etc/nginx/conf.d/resolvedor.inc

servidor="$(awk '/^nameserver/ { print $2; exit }' /etc/resolv.conf 2>/dev/null || true)"

if [ -z "$servidor" ]; then
    echo "detectar-dns: nenhum nameserver em /etc/resolv.conf, usando 127.0.0.11" >&2
    servidor=127.0.0.11
fi

echo "resolver ${servidor} valid=10s ipv6=off;" > "$DESTINO"
echo "resolver_timeout 3s;" >> "$DESTINO"

echo "detectar-dns: resolvedor definido como ${servidor}"
