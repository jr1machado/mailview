# Política de segurança do MailView

O MailView é um fork independente. Relatórios enviados ao projeto de origem não
são automaticamente encaminhados aos mantenedores do MailView.

## Versões suportadas

| Versão | Suporte de segurança |
|---|---|
| `v0.5.x` | Sim |
| `< v0.5.0` | Não |

## Como reportar

Não publique vulnerabilidades exploráveis em issues. Use o recurso **Security →
Report a vulnerability** do repositório MailView para abrir um advisory privado.
Inclua versão, ambiente, impacto, pré-condições, passos mínimos de reprodução e,
quando possível, uma sugestão de correção.

Evite incluir dados pessoais, credenciais ou conteúdo real de mensagens. A equipe
fará a triagem, confirmará o escopo e coordenará a divulgação. Dependências e
trechos herdados continuam sujeitos às correções do upstream, mas a decisão de
incorporação e o calendário de release pertencem exclusivamente ao MailView.

## Limites de responsabilidade

Configuração insegura de SMTP, DNS, proxy, storage, PostgreSQL ou provedores
externos não é corrigida automaticamente pela aplicação. Consulte
`docs/REFERENCIA_OPERACIONAL.md` e `ISSUES_CONHECIDOS.md` antes do deploy.
