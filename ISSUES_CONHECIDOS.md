# MailView v0.5.0 — Issues conhecidos

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
- Billing accounts, subscriptions e invoices têm modelo tenant/RLS; o dashboard deriva MRR/ARR das invoices pagas, mas gateway, cobrança recorrente e conciliação não estão implementados.
- `dedicated_requested`/`dedicated` valida e publica o contrato de roteamento; não provisiona fisicamente banco, SMTP, storage, worker, KMS ou namespace.
- SMTP profiles tenant-scoped têm modelo persistente, mas o campaign manager ainda usa a configuração SMTP herdada até o dispatcher da próxima fase.
- Verificação e revalidação DNS são automáticas; emissão ACME permanece no proxy/controller externo, que reporta o estado TLS pela API.
- Webhooks, exports e mensagens transacionais da Fase 3 têm schema/isolamento, mas seus dispatchers continuam no roadmap.
- O workflow formal de campanha existe como sidecar/API e sincroniza estados executáveis com o core; o editor herdado ainda não oferece todos os botões de review/approval/reject.
- API keys MailView podem ser criadas/revogadas com hash seguro, mas a autenticação HTTP ainda usa os mecanismos herdados até o middleware MailView consumir `mv_api_keys`.
- Governança de contatos (tags/consentimento/supressão) possui API própria; os campos ainda não aparecem em todos os formulários herdados.
- Incidentes possuem API e contagem no dashboard, mas ainda não há tela completa de triagem no portal.

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

- O wizard valida ownership TXT/CNAME. SPF, DKIM, DMARC, CNAME de tracking, reputação, warm-up, throttling e feedback loops continuam dependentes da operação/provedor.
- Não há spam score, reputação automática, suppressions globais externas ou otimização por IA.
- Webhooks dependem da configuração correta de assinatura/secret de cada provedor.

## Release e plataformas

- Artefatos para sistemas/arquiteturas listados no GoReleaser são cross-compilados; esta release não declara que todas as combinações receberam teste E2E nativo.
- O frontend preserva avisos de depreciação Sass e chunks grandes no build; não impedem o funcionamento, mas pedem modernização.
- Documentação histórica sob `docs/docs/content` ainda usa a marca upstream; serve como referência do core. Os documentos no topo de `docs/` prevalecem.

Relate defeitos novos separando reprodução, host tenant/legado, versão/tag, role do banco e logs sem segredos.
