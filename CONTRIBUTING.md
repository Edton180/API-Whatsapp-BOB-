# Como contribuir com a API WhatsApp BOB

Criador e mantenedor: **Edton Kaik Braz Da Conceição** ([@Edton180](https://github.com/Edton180)).

A BOB é livre e aberta. Toda ajuda conta: correção de bug, nova função, teste, documentação ou tradução.

## Passo a passo

1. Abra uma [issue](https://github.com/Edton180/API-Whatsapp-BOB-/issues) descrevendo o bug ou a ideia.
2. Faça um fork e crie um ramo: `feat/nome-da-funcao`, `fix/descricao`, `docs/assunto`.
3. Escreva o código em Go com `gofmt`, e um teste para o que você mudou.
4. Rode `go build ./cmd/evolution-go` e `go test ./pkg/... -run Bob`.
5. Use commits no padrão *Conventional Commits* (`feat:`, `fix:`, `docs:`...).
6. Envie o pull request para a `main` deste repositório.

## Regras do projeto

- **Código da Evolution Go:** prefira colocar melhorias em arquivos novos com prefixo `bob_` em vez de reescrever arquivos do motor. Isso deixa a sincronização com o oficial ([docs/sincronizacao.md](docs/sincronizacao.md)) sem conflitos.
- **Licença:** contribuições entram sob a mesma licença do projeto ([LICENSE](LICENSE)). Mantenha os avisos de copyright existentes e o [NOTICE](NOTICE).
- **Marca:** não adicione logos, cores ou o painel da Evolution Go ([TRADEMARKS.md](TRADEMARKS.md)).
- **Mecanismo de licença da Evolution Go:** não remova nem contorne. Ele faz parte das condições do projeto oficial.
- **Nada de rota de mentira:** uma rota só entra se fizer o que diz (por exemplo, enviar a mensagem de verdade).

## Apoie

Se a BOB ajuda o seu trabalho, considere uma doação por Pix (veja o [README](README.md#apoie-o-projeto-pix)).
