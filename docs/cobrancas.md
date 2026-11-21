# Cobranças

A BOB cobra os clientes pelo WhatsApp: monta a cobrança (Pix copia e cola e QR Code, link de
pagamento ou boleto), entrega ao cliente, guarda a situação e avisa por evento. O dinheiro vai
direto para a conta de quem cobra — a BOB não intermedia pagamento e não guarda credencial de
gateway.

Dois jeitos de montar a cobrança:

1. **Pix da própria instância** (nativo): com a chave Pix cadastrada, a BOB gera o BR Code com o
   valor e um identificador por cobrança. Não precisa de gateway nem de conta em serviço nenhum.
2. **Cobrança de um gateway**: o seu sistema cria a cobrança no gateway (Asaas, Mercado Pago,
   Stripe, Efí, OpenPix...) e manda para a BOB o `pixCode`, o `paymentUrl` e/ou o `boletoLine`
   já prontos, com `reference` sendo o id da cobrança lá. A BOB cuida da entrega e do
   acompanhamento.

Todas as rotas usam o token da instância no cabeçalho `apikey` (ou a chave global com o cabeçalho
`instance`). Cada instância só vê e mexe nas suas cobranças.

## Pix da instância

```http
PUT /charge/settings
{"pixKey": "contato@loja.com", "pixKeyType": "email", "pixName": "Loja do Zé", "pixCity": "Manaus",
 "message": "Olá {nome}! Sua {descricao} de {valor} vence em {vencimento}.",
 "receiptMessage": "Recebemos {valor}. Obrigado!"}
```

- `pixKeyType`: `cpf`, `cnpj`, `phone`, `email`, `random` ou vazio (a BOB descobre pelo formato).
- `pixName` (até 25 letras) e `pixCity` (até 15) vão no BR Code; sem cidade, vai `BRASIL`.
- `message` e `receiptMessage` são opcionais e aceitam `{nome}`, `{descricao}`, `{valor}` e
  `{vencimento}`. Vazios, a BOB usa os textos padrão.
- A chave é conferida na hora de salvar (`GET /charge/settings` devolve o que está valendo).

## Criar e enviar

```http
POST /charge/create
{"number": "5511999998888", "customerName": "Maria", "description": "Mensalidade de novembro",
 "amount": 49.90, "dueDate": "2026-11-20", "reference": "fatura-123", "send": true}
```

| Campo | Para que serve |
|---|---|
| `number` | número do cliente com DDI e DDD (obrigatório) |
| `amount` / `amountCents` | valor em reais (`49.90`) ou em centavos (`4990`) — um dos dois |
| `dueDate` | vencimento `AAAA-MM-DD` (opcional) |
| `pixCode` | Pix copia e cola pronto de um gateway (o CRC é conferido) |
| `paymentUrl` | link de pagamento (cartão, checkout) |
| `boletoLine` | linha digitável do boleto |
| `reference` | id da cobrança no seu sistema: repetir a chamada devolve a mesma cobrança |
| `metadata` | o que você quiser guardar (volta nas consultas e nos eventos) |
| `send` | `false` só cria, sem enviar (padrão: envia) |
| `message`, `delay` | texto no lugar do padrão e espera de "digitando..." |

Sem `pixCode`, `paymentUrl` e `boletoLine`, a BOB gera o Pix com a chave da instância (só em BRL).
Com `send`, o cliente recebe o QR Code com o texto na legenda e, em seguida, o copia e cola numa
mensagem só dele (fica fácil de copiar no celular). Se o envio falhar, a cobrança fica gravada e a
resposta traz `sendError` — com a instância desconectada, a BOB responde 409 sem gravar nada.

## Acompanhar, lembrar e dar baixa

| Rota | O que faz |
|---|---|
| `GET /charge/list?status=&number=&limit=&offset=` | lista; `status`: `pending`, `paid`, `cancelled` ou `overdue` |
| `GET /charge/{id}` | uma cobrança, pelo id da BOB ou pela sua `reference` |
| `GET /charge/{id}/qrcode` | imagem PNG do QR Code do Pix |
| `POST /charge/{id}/send` | reenvia (lembrete); aceita `message` e `delay` |
| `POST /charge/{id}/pay` | dá baixa e avisa o cliente (`notify: false` não avisa) |
| `POST /charge/{id}/cancel` | cancela |

A baixa vale uma vez só: chamar `/pay` de novo responde `changed: false` e não avisa o cliente
outra vez. Cobrança paga não é cancelada nem reenviada (409), e cancelada não é paga.

Quem confirma o pagamento é quem recebe o dinheiro: no Pix da instância, o aviso do banco ou a
conferência do extrato; numa cobrança de gateway, o webhook do gateway. Nos dois casos, o seu
sistema chama `POST /charge/{reference}/pay` e a BOB avisa o cliente e registra a baixa.

## Eventos

Categoria `CHARGE` (veja [eventos.md](eventos.md)): `ChargeCreated`, `ChargeSent`, `ChargePaid` e
`ChargeCancelled`. O `data` é a cobrança inteira, igual à resposta das rotas.

## Painel

A aba **Cobranças** cadastra o Pix da instância, cria cobranças, copia o Pix copia e cola, reenvia
lembretes, dá baixa e cancela.
