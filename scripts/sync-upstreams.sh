#!/usr/bin/env bash
set -e

echo "======================================================================"
echo "🔄 SINCRONIZADOR OFICIAL API WHATSAPP BOB"
echo "   1) Motor Go: https://github.com/evolution-foundation/evolution-go"
echo "   2) Funções API: https://github.com/evolution-foundation/evolution-api"
echo "======================================================================"

if ! git remote | grep -q "^upstream-go$"; then
    git remote add upstream-go https://github.com/evolution-foundation/evolution-go.git
fi
if ! git remote | grep -q "^upstream-api$"; then
    git remote add upstream-api https://github.com/evolution-foundation/evolution-api.git
fi

echo "📥 [1/3] Buscando atualizações do motor Evolution Go (upstream-go)..."
git fetch upstream-go main

echo "📥 [2/3] Buscando atualizações de endpoints e integrações da Evolution API (upstream-api)..."
git fetch upstream-api main

echo "🔀 [3/3] Aplicando atualizações do motor Go sem sobrescrever os pacotes exclusivos da API WhatsApp BOB..."
git merge upstream-go/main --no-commit --no-ff || true

git checkout HEAD -- README.md CONTRIBUTING.md scripts/ 2>/dev/null || true

echo ""
echo "📊 Últimos commits lançados na Evolution API (para referência de paridade):"
git log upstream-api/main -n 5 --oneline || true

echo ""
echo "✅ Sincronização concluída! Para salvar e atualizar o repositório oficial e o OmniFlow:"
echo "   git add ."
echo "   git commit -m 'chore(sync): atualiza motor evolution-go e paridade evolution-api'"
echo "   git push origin main"
