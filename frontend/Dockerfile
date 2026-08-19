# Multi-stage: o Node e as ~600MB de node_modules ficam no estagio de
# construcao. A imagem final carrega apenas os arquivos estaticos e o nginx.

FROM docker.io/library/node:22-alpine AS construcao

WORKDIR /app

# package.json e package-lock.json primeiro: enquanto as dependencias nao
# mudam, esta camada vem do cache e o build nao reinstala tudo a cada edicao.
COPY package.json package-lock.json ./

# `npm ci` instala exatamente o que esta no lock, sem resolver versoes de novo.
# E o que garante que a imagem tenha as mesmas dependencias da maquina de quem
# desenvolveu.
RUN npm ci

COPY . .
RUN npm run build

FROM docker.io/library/nginx:1.27-alpine

COPY --from=construcao /app/dist/frontend/browser /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY proxy-comum.inc /etc/nginx/conf.d/proxy-comum.inc

# A imagem oficial executa /docker-entrypoint.d/*.sh, em ordem, antes de subir o
# nginx. E onde o endereco do DNS interno e descoberto e gravado.
COPY 20-detectar-dns.sh /docker-entrypoint.d/20-detectar-dns.sh
RUN chmod +x /docker-entrypoint.d/20-detectar-dns.sh

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s \
    CMD wget -qO- http://127.0.0.1:8080/health || exit 1
