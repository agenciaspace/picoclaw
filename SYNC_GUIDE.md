# 🔄 Mantendo PicoClaw Atualizado Localmente

Guia para sincronizar mudanças com a branch de desenvolvimento.

---

## 📋 Quick Commands

### Atualizar tudo (recomendado):
```bash
make sync-dev
# OR
git fetch origin claude/hostinger-remote-deployment-TGVof
git merge origin/claude/hostinger-remote-deployment-TGVof
```

### Ver mudanças antes de aplicar:
```bash
git fetch origin claude/hostinger-remote-deployment-TGVof
git diff origin/claude/hostinger-remote-deployment-TGVof
```

### Resetar para a branch remota (se fez muitas mudanças locais):
```bash
git fetch origin claude/hostinger-remote-deployment-TGVof
git reset --hard origin/claude/hostinger-remote-deployment-TGVof
```

---

## 🔍 Entendendo o Fluxo de Trabalho

```
Local (seu computador)
    ↓
    ├─ Branch: claude/hostinger-remote-deployment-TGVof
    │  (sua branch de trabalho)
    │
Remote (GitHub)
    ↓
    ├─ Branch: claude/hostinger-remote-deployment-TGVof
    │  (repositório central)
    │
Hostinger VPS
    ↓
    ├─ /opt/picoclaw (aplicação rodando)
    │  (sincronizado via GitHub Actions)
```

---

## 📝 Cenários Comuns

### Cenário 1: Sincronizar após fazer mudanças locais

```bash
# 1. Ver status
git status

# 2. Fazer commit das mudanças
git add .
git commit -m "chore: my local changes"

# 3. Puxar mudanças do repositório remoto
git pull origin claude/hostinger-remote-deployment-TGVof

# 4. Se tiver conflitos:
# - Editar os arquivos com conflito
# - Resolver manualmente
git add <arquivo-resolvido>
git commit -m "resolve merge conflicts"

# 5. Enviar suas mudanças
git push origin claude/hostinger-remote-deployment-TGVof
```

---

### Cenário 2: Sincronizar SEM fazer mudanças

```bash
# Simples: puxar tudo
git pull origin claude/hostinger-remote-deployment-TGVof

# Ou de forma mais segura:
git fetch origin claude/hostinger-remote-deployment-TGVof
git merge origin/claude/hostinger-remote-deployment-TGVof
```

---

### Cenário 3: Voltar para versão anterior (se algo deu errado)

```bash
# Ver histórico
git log --oneline -10

# Voltar para um commit específico (CUIDADO: descarta mudanças recentes)
git reset --hard <COMMIT_HASH>

# Ou simplesmente resetar para a versão remota
git reset --hard origin/claude/hostinger-remote-deployment-TGVof
```

---

## ⚙️ Arquivos Importantes (Não Edite Diretamente)

Esses arquivos são gerenciados automaticamente pelo Claude Code. **Edite apenas via scripts**:

| Arquivo | Como Editar |
|---------|-------------|
| `.github/workflows/deploy-hostinger.yml` | `make setup-telegram` / `make setup-tailscale` |
| `deploy/hostinger/setup-server.sh` | `make setup-tailscale` |
| `deploy/hostinger/setup-telegram.sh` | `make setup-telegram` |
| `deploy/hostinger/docker-compose.production.yml` | Manual via SSH |
| `Makefile` | `make` targets são auto-gerenciados |

---

## 🚨 Evite Fazer Isso

### ❌ NÃO edite arquivos manualmente que podem ter conflitos:

```bash
# RUIM: Editar workflow manualmente
nano .github/workflows/deploy-hostinger.yml

# BOM: Usar os scripts
make setup-telegram
make setup-tailscale
```

### ❌ NÃO faça force push para main:

```bash
# MUITO RUIM - pode apagar trabalho de outros!
git push --force-with-lease origin main

# OK para sua branch de dev
git push --force-with-lease origin claude/hostinger-remote-deployment-TGVof
# (só se tiver certeza)
```

### ❌ NÃO commit .env ou secrets:

```bash
# Se acidentalmente fez commit de secrets:
git rm --cached .env config/.env
git commit -m "remove secrets (they were already in GitHub Secrets anyway)"
git push origin claude/hostinger-remote-deployment-TGVof
```

---

## ✅ Workflow Recomendado

### Diariamente:

```bash
# Ao iniciar o dia
git fetch origin
git status

# Se houver mudanças remotas
git pull origin claude/hostinger-remote-deployment-TGVof
```

### Antes de Fazer Mudanças:

```bash
# Garantir que está atualizado
git pull origin claude/hostinger-remote-deployment-TGVof

# Criar sua mudança
# ... editar arquivos ...

# Commitar
git add .
git commit -m "feat: descrição da mudança"

# Enviar
git push origin claude/hostinger-remote-deployment-TGVof
```

### Após Deploy no Hostinger:

```bash
# Verificar se o deploy funcionou
git log --oneline -5

# Ver status do deploy
# (GitHub Actions mostra o status automaticamente)
```

---

## 🔧 Comandos Git Úteis

```bash
# Ver qual branch está
git branch -v

# Ver mudanças não commitadas
git diff

# Ver histórico
git log --oneline -10

# Ver diferenças com remoto
git diff origin/claude/hostinger-remote-deployment-TGVof

# Limpar arquivos não rastreados
git clean -fd

# Descartar mudanças em um arquivo
git checkout -- <arquivo>

# Descartar todas mudanças locais
git reset --hard HEAD
```

---

## 📊 Monitorando Deploy

Após fazer push, o deploy automático começa. Monitore em:

**GitHub Actions:**
```
https://github.com/agenciaspace/picoclaw/actions
```

**Ou via terminal:**
```bash
gh run list -b claude/hostinger-remote-deployment-TGVof --limit 5

# Ver logs de um deploy específico
gh run view <RUN_ID>
```

---

## 🐛 Resolving Merge Conflicts

Se tiver conflitos ao fazer pull:

```bash
# 1. Ver quais arquivos têm conflito
git status

# 2. Editar os arquivos com conflito
# Procurar por:
# <<<<<<< HEAD      (sua versão local)
# =======
# >>>>>>> origin/... (versão remota)

# 3. Decidir qual versão manter ou combinar

# 4. Marcar como resolvido
git add <arquivo-resolvido>

# 5. Completar merge
git commit -m "resolve merge conflicts"
```

---

## 📦 Atualizar Dependências

```bash
# Ver dependências desatualizadas
make check

# Atualizar todas
make update-deps

# Commitar mudanças
git add go.mod go.sum
git commit -m "chore: update dependencies"
git push origin claude/hostinger-remote-deployment-TGVof
```

---

## 🔐 Segurança

### NUNCA commit secrets:
- ❌ Bot tokens de Telegram
- ❌ API keys
- ❌ Passwords
- ❌ Private keys

### Use GitHub Secrets:
```bash
# Adicionar secret
gh secret set PICOCLAW_TELEGRAM_BOT_TOKEN -b "sua_token"

# Ver secrets (valores não aparecem)
gh secret list
```

---

## 📱 Sincronizar em Múltiplas Máquinas

Se trabalhar em vários computadores:

```bash
# Máquina A: Fazer mudanças e push
git add .
git commit -m "feat: minha mudança"
git push origin claude/hostinger-remote-deployment-TGVof

# Máquina B: Puxar mudanças
git pull origin claude/hostinger-remote-deployment-TGVof
```

---

## 🎯 Checklist de Atualização

- [ ] `git pull origin claude/hostinger-remote-deployment-TGVof`
- [ ] `git status` (verifica se há conflitos)
- [ ] Testar localmente: `make build && make run`
- [ ] Conferir arquivos: `git diff HEAD~1` (mudanças do último commit)
- [ ] `git push origin claude/hostinger-remote-deployment-TGVof`
- [ ] Monitorar GitHub Actions (deploy automático)
- [ ] Testar no Hostinger após deploy

---

## ❓ FAQ

**P: Como saber se está desatualizado?**
R: `git fetch` e depois `git status` mostra se há mudanças remotas.

**P: Posso fazer push diretamente para main?**
R: Tecnicamente sim, mas evite. Use a branch claude/hostinger-* para segurança.

**P: O que fazer se acidentalmente editei arquivo importante?**
R: `git checkout -- <arquivo>` para descartar mudanças.

**P: Como reverter um commit já feito?**
R: `git revert <COMMIT_HASH>` (cria novo commit que desfaz as mudanças).

**P: Preciso fazer pull toda vez?**
R: Sim, para manter sincronizado. Especialmente antes de fazer push.

---

**Dica:** Faça `git pull` regularmente para evitar conflitos grandes! 🚀
