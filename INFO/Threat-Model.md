# Threat model inicial — MailView

**Escopo:** Fase 0  
**Ativos prioritários:** PII dos contatos, credenciais SMTP, chaves de API, segredos MFA, conteúdo de campanhas, domínios e trilha de auditoria.

| Ameaça | Vetor provável | Impacto | Controle obrigatório | Fase |
|---|---|---|---|---|
| Vazamento entre tenants | Query sem escopo, IDOR, job reutilizado | Crítico | Contexto transacional, RLS e testes cross-tenant | 2 |
| Takeover de domínio | Host header ou DNS não validado | Crítico | Prova DNS, normalização de host e revalidação | 4 |
| Tomada de conta administrativa | Senha vazada ou brute force | Crítico | MFA, rate limit, sessões rotativas e step-up | 1 |
| Abuso de SMTP | Credencial ou perfil de outro tenant | Alto | Perfil por tenant, quotas e auditoria | 4–5 |
| Exportação indevida | Permissão excessiva ou API key vazada | Alto | RBAC, scopes, step-up e audit trail | 1–2 |
| SSRF por webhook | URL controlada pelo tenant | Alto | Allowlist/egress control, DNS rebinding protection e timeouts | 4 |
| XSS em campanha/template | HTML malicioso | Alto | Sanitização, CSP e isolamento de preview | 3 |
| Segredo exposto | Git, imagem ou variável de ambiente | Alto | Docker secrets, secret scan e `.dockerignore` | 0 |
| Comprometimento de dependência | Dependência ou imagem vulnerável | Alto | Versões fixadas, SBOM e scan de dependências | 0 |

## Invariantes de segurança

1. Uma identidade autenticada só opera um tenant resolvido pelo servidor.
2. O banco é a barreira final de isolamento; filtros de aplicação não bastam.
3. Dados e segredos nunca entram em logs, imagens ou repositório.
4. Operações administrativas, exportações e mudanças de segurança são auditáveis.
5. Jobs carregam tenant, ator e idempotency key, todos validados antes de processar recursos.

## Critérios para avançar da Fase 0

- imagens e dependências verificadas no CI;
- nenhuma configuração de produção com segredos padrão;
- ADRs e esta matriz revisados junto de cada mudança de arquitetura;
- contrato de migration próprio definido antes da primeira tabela SaaS.
