# Receber as atualizações da Evolution Go

A BOB guarda em [`UPSTREAM`](../UPSTREAM) o commit da Evolution Go oficial que ela já contém. Ao sincronizar, só a diferença entre esse commit e o código novo é aplicada, com `git apply --3way`. Assim as melhorias da BOB continuam e só as partes que mudaram no projeto oficial entram.

## Automático (GitHub)

O workflow [`sincronizar-evolution.yml`](../.github/workflows/sincronizar-evolution.yml) roda todo dia e também pode ser iniciado em **Actions → Sincronizar Evolution Go → Run workflow**. Quando há novidade, ele abre um pull request com as mudanças; o CI compila e testa antes do merge.

## Manual

```bash
bash scripts/sincronizar-evolution.sh          # último código oficial
bash scripts/sincronizar-evolution.sh 0.7.3    # até uma tag ou commit
```

| Saída | Significado |
| --- | --- |
| `nada-novo` | Já está igual ao oficial. |
| `aplicado <de> <para>` | Mudanças aplicadas; revise, teste e faça o commit. |
| `conflito <de> <para>` | O mesmo trecho mudou nos dois lados; resolva os marcadores `<<<<<<<` e faça o commit. |

O atalho antigo `scripts/sync-upstreams.sh` chama este mesmo script.

## O que nunca entra

O painel web (`manager/`), as imagens de marca (`public/`), o binário compilado, o README e os workflows oficiais, por causa da política de marca ([TRADEMARKS.md](../TRADEMARKS.md)).

## Evolution API

A [Evolution API](https://github.com/evolution-foundation/evolution-api) é escrita em TypeScript e serve de referência de funções. Não há merge de código dela: cada função é reimplementada em Go, com atribuição no [NOTICE](../NOTICE) quando a lógica for adaptada.
