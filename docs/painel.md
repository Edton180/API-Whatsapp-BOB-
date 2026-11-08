# Painel web

O painel fica em `http://SEU-SERVIDOR:PORTA/painel/` e já vem dentro do programa (não precisa instalar nada).
Entre com a `GLOBAL_API_KEY`. Marque "Lembrar neste navegador" só em computador de confiança.

| Tela | O que faz |
|---|---|
| **Instâncias** | Lista os números com o estado (conectada, conectando, desconectada), número, webhook e eventos. Cria, conecta, reconecta, desconecta, sai do WhatsApp e exclui. |
| **Conectar** | QR Code atualizado sozinho, ou código de 8 caracteres para digitar no celular (sem câmera). |
| **Configurar › Webhook** | URL e eventos enviados. |
| **Configurar › Comportamento** | Recusar ligações (com mensagem), marcar como lida, sempre online, ignorar grupos e status. |
| **Configurar › Integrações** | Chatwoot, Typebot e N8N. Os segredos nunca voltam para a tela; em branco, o salvo é mantido. |
| **Configurar › Proxy** | Proxy só desta instância. |
| **Configurar › Acesso** | ID, token e exemplo de envio com `curl`. |
| **Registro** | O que aconteceu com a instância desde o último início (conexão, erros, ligações recusadas). |
| **Enviar mensagem** | Teste de texto, mídia, figurinha, localização e enquete. |
| **Chamadas** | Liga as chamadas de voz e vídeo na instância, mostra as chamadas tocando (atender, atender com vídeo, recusar, puxar para este navegador, desligar), softphone para ligar para um número, microfone, câmera e o histórico (perdidas, atendidas, recusadas). Veja [chamadas.md](chamadas.md). |
| **Eventos ao vivo** | Tudo o que chega e sai, em tempo real (WebSocket). |
| **Servidor** | Versão, commit, tempo no ar, memória, banco, ffmpeg e links. |
| **Documentação da API** | Todas as rotas com exemplos, em `/swagger/index.html`. |

Para desligar o painel (API só por código), use `PANEL_ENABLED=false`.

As capturas de tela em [imagens/telas](imagens/telas) foram feitas com dados de exemplo.
