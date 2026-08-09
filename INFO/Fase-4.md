# 10. RBAC

## 10.1 Papéis padrão do tenant

### Tenant Owner

- controle total do tenant;
- billing;
- domínios;
- usuários;
- exclusão;
- exportações.

### Tenant Admin

- usuários;
- campanhas;
- listas;
- templates;
- remetentes;
- relatórios;
- sem excluir tenant ou alterar ownership.

### Campaign Manager

- criar, editar, aprovar e agendar campanhas;
- administrar templates;
- visualizar métricas.

### Operator

- criar rascunhos;
- importar contatos;
- executar tarefas operacionais;
- sem publicar campanha sem aprovação.

### Analyst

- visualizar relatórios;
- exportar métricas autorizadas;
- sem alterar campanhas.

### Viewer

- somente leitura.

### Billing Manager

- plano;
- faturas;
- consumo;
- sem acesso a contatos.

## 10.2 Papéis da plataforma

- Platform Super Admin;
- Platform Operations;
- Platform Support;
- Platform Security;
- Platform Billing;
- Platform Auditor.

## 10.3 Permissões granulares

Formato:

```text
resource.action.scope
```

Exemplos:

```text
campaign.create.tenant
campaign.approve.tenant
subscriber.export.tenant
domain.manage.tenant
user.invite.tenant
audit.read.tenant
smtp.manage.tenant
```

## 10.4 Regras

- permissões são aditivas;
- negação explícita deve prevalecer;
- papéis customizados por tenant em plano premium;
- nenhuma permissão global pode ser criada por tenant;
- mudanças de papel invalidam sessões sensíveis.

---

# 11. Portal administrativo

## 11.1 Dashboard global

- tenants ativos;
- MRR/ARR;
- e-mails enviados;
- taxa de erro;
- filas;
- bounces;
- reclamações;
- incidentes;
- consumo de infraestrutura;
- reputação de domínios;
- falhas de webhooks.

## 11.2 Gestão de tenants

- criar;
- editar;
- suspender;
- reativar;
- migrar para dedicado;
- aplicar quota;
- ver auditoria;
- redefinir owner;
- iniciar offboarding.

## 11.3 Impersonation

Permitida apenas para suporte autorizado.

Requisitos:

- justificativa obrigatória;
- expiração curta;
- banner visível;
- MFA recente;
- aprovação opcional;
- log imutável;
- proibição de visualizar segredos;
- proibição de alterar billing sem privilégio específico.

---

# 12. Portal do cliente

## 12.1 Home

- volume enviado;
- campanhas ativas;
- contatos;
- bounces;
- consumo do plano;
- domínios pendentes;
- alertas de entregabilidade.

## 12.2 Campanhas

Estados:

```text
draft -> review -> approved -> scheduled -> sending -> completed
                   \-> rejected
```

Controles:

- teste de envio;
- preview desktop/mobile;
- spam-check opcional;
- validação de links;
- aprovação de campanha;
- agendamento;
- cancelamento seguro;
- idempotência.

## 12.3 Contatos

- importação CSV;
- deduplicação por tenant;
- campos customizados;
- consentimento;
- tags;
- listas;
- supressão;
- exportação controlada.

## 12.4 Domínios

Wizard com:

- domínio;
- finalidade;
- registros DNS;
- verificação;
- SPF;
- DKIM;
- DMARC;
- CNAME de tracking;
- teste final.

---

# 13. Segurança de dados

## 13.1 Criptografia

Em trânsito:

- TLS 1.2 mínimo;
- TLS 1.3 preferencial;
- HSTS;
- mTLS entre serviços críticos em ambientes maduros.

Em repouso:

- criptografia de disco;
- segredos TOTP criptografados em nível de campo;
- chaves de API armazenadas apenas como hash;
- credenciais SMTP criptografadas;
- backups criptografados.

## 13.2 Chaves

- envelope encryption;
- chave mestra fora do banco;
- rotação programada;
- chave por tenant no plano Enterprise;
- versionamento de chave.

## 13.3 Dados sensíveis

Classificação mínima:

- PII;
- credenciais;
- segredos;
- conteúdo de campanha;
- dados de billing;
- logs de auditoria.
