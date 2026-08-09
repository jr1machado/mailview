# MailView v0.4.0 — Issues conhecidos

Limitações confirmadas desta release; roadmap não deve ser confundido com recurso entregue.

## Operação e escala

- O binário é monolítico. `--passive` desliga o scanner de campanhas, mas não existe worker dedicado, eleição de leader ou scheduler distribuído.
- Import em `processing` não é retomado automaticamente após reinício; pode exigir correção operacional do job.
- Cancelamento de import ocorre entre lotes, não interrompe uma transação em curso.
- Filesystem em múltiplos nós exige volume compartilhado; S3 é recomendado.
- Compose de referência não fornece HA, backup, autoscaling ou PostgreSQL replicado.
- Não há métricas Prometheus/OpenTelemetry próprias nem SLO/SLA publicado.

## Produto e multi-tenancy

- Planos, quotas e usage são modelo/gestão; limites não bloqueiam contatos, envios, domínios ou seats.
- Billing, invoices, MRR/ARR e gateway de pagamento não estão implementados.
- `dedicated_requested`/`dedicated` registra intenção/estado; não provisiona banco, SMTP, storage, worker ou namespace.
- SMTP/settings continuam globais. RBAC expõe `smtp.manage.tenant`, mas não há perfil SMTP isolado por tenant.
- Verificação de domínio é manual; não consulta DNS nem provisiona TLS automaticamente por tenant.
- Não há workflow formal separado de review/approval, embora permissões de campanha existam.

## Importação e dados

- Import MailView aceita CSV simples com `email` e `name`; ZIP, mapeamento avançado e atributos ficam no importador legado e não são oferecidos no modo tenant.
- HMAC usa uma chave por instalação, sem rotação/versionamento de chave.
- Upload possui limite de tamanho, mas não quota/rate limit dedicado por tenant.
- Arquivos importados precisam de política operacional de retenção/limpeza.

## Identidade e segurança

- OIDC autocriado não cria automaticamente membership MailView.
- Auditoria é append-only na aplicação, mas não é imutável contra DBA/superuser.
- A ponte de Super Admin herdada continua aceita em parte do Control Plane; papéis de plataforma refinam funções sensíveis, mas a separação completa do RBAC core ainda é compatibilidade em evolução.
- Prefixo `LISTMONK_*`, módulo Go upstream e nomes do database/role do Compose local permanecem por compatibilidade técnica.

## Entregabilidade

- SPF, DKIM, DMARC, reputação, warm-up, throttling do provedor e feedback loops dependem da operação/provedor.
- Não há spam score, reputação automática, suppressions globais externas ou otimização por IA.
- Webhooks dependem da configuração correta de assinatura/secret de cada provedor.

## Release e plataformas

- Artefatos para sistemas/arquiteturas listados no GoReleaser são cross-compilados; esta release não declara que todas as combinações receberam teste E2E nativo.
- O frontend preserva avisos de depreciação Sass e chunks grandes no build; não impedem o funcionamento, mas pedem modernização.
- Documentação histórica sob `docs/docs/content` ainda usa a marca upstream; serve como referência do core. Os documentos no topo de `docs/` prevalecem.

Relate defeitos novos separando reprodução, host tenant/legado, versão/tag, role do banco e logs sem segredos.
