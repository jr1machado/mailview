# Deploy de produção

Copie `.env.example` para `.env`, substitua apenas as referências de imagem pelos artefatos imutáveis publicados pelo pipeline e crie os arquivos em `secrets/` fora do Git.

```sh
cd deploy
cp .env.example .env
mkdir -p secrets
chmod 700 secrets
docker compose --env-file .env -f compose.production.yml config
docker compose --env-file .env -f compose.production.yml up -d
```

O Compose é uma topologia de segurança para a Fase 0; `frontend`, `api` e `worker` serão disponibilizados pelas fases seguintes. Não reutilize o `docker-compose.yml` da raiz em produção: ele é o exemplo de desenvolvimento herdado do Listmonk.
