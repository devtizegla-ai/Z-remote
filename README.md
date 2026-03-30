# Remote Access MVP (Cliente-Servidor)

MVP funcional de acesso remoto assistido, com backend em Go + SQLite e cliente desktop multiplataforma em Tauri + HTML/CSS/JS puro.

## 1. Visão Geral

Este projeto implementa um fluxo básico de acesso remoto com arquitetura cliente-servidor:

- Autenticação por e-mail/senha com JWT.
- Registro de dispositivos e status online/offline.
- Solicitação de sessão entre dispositivos do mesmo usuário.
- Aceite/rejeição explícitos no dispositivo receptor.
- Criação de sessão remota com token temporário.
- Sinalização e relay simplificado via WebSocket.
- Compartilhamento de tela básico do host.
- Envio de eventos de mouse/teclado do controlador.
- Transferência simples de arquivos em sessão.
- Logs básicos de auditoria.

## 2. Estrutura de Diretórios

```text
.
├── server
│   ├── cmd/server
│   ├── internal
│   │   ├── auth
│   │   ├── config
│   │   ├── devices
│   │   ├── files
│   │   ├── http
│   │   ├── models
│   │   ├── sessions
│   │   ├── storage
│   │   └── ws
│   ├── migrations
│   ├── scripts
│   ├── .env.example
│   ├── Dockerfile
│   └── go.mod
├── client
│   ├── src
│   │   ├── css
│   │   └── js/modules
│   ├── src-tauri
│   ├── scripts
│   ├── .env.example
│   ├── package.json
│   └── vite.config.js
├── render.yaml
└── README.md
```

## 3. Requisitos

### Backend

- Go 1.23+
- Acesso de escrita em `server/data`

### Cliente

- Node.js 20+
- Rust toolchain estável
- Tauri prerequisites do SO (WebView2 no Windows, etc.)

## 4. Configuração de Ambiente

### 4.1 Backend (`server/.env`)

Copie `server/.env.example` para `server/.env` e ajuste:

- `SERVER_ADDR=:8080`
- `JWT_SECRET=<segredo forte>`
- `DB_DRIVER=sqlite`
- `DB_URL=./data/remote_access.db`
- `CORS_ALLOWED_ORIGINS=http://localhost:1420`
- `FILE_MAX_BYTES=10485760`
- `FILE_STORAGE_DIR=./data/uploads`

### 4.2 Cliente (`client/.env`)

Copie `client/.env.example` para `client/.env` e ajuste:

- `VITE_SERVER_URL=http://localhost:8080`
- `VITE_APP_VERSION=0.1.0`

## 5. Como Rodar Localmente

## 5.1 Subir Backend

```powershell
cd server
copy .env.example .env
go run ./cmd/server
```

Scripts auxiliares:

```powershell
./scripts/dev.ps1
./scripts/build.ps1
```

Health check:

```bash
GET http://localhost:8080/health
```

## 5.2 Seed opcional para ambiente dev

Cria usuário/dispositivos de teste:

```powershell
cd server
go run ./scripts/seed
```

Ou habilite auto-seed no boot com:

- `SEED_DEV_DATA=true` no `server/.env`

Credenciais seed:

- Email: `dev@example.com`
- Senha: `dev123456`

## 5.3 Subir Cliente Desktop

```powershell
cd client
copy .env.example .env
npm install
npm run dev
```

## 6. API REST (MVP)

### Públicos

- `POST /api/auth/register`
- `POST /api/auth/login`
- `GET /health`

### Autenticados (Bearer JWT)

- `GET /api/me`
- `POST /api/devices/register`
- `GET /api/devices`
- `POST /api/sessions/request`
- `POST /api/sessions/respond`
- `POST /api/sessions/start`
- `GET /api/sessions`
- `POST /api/files/upload`
- `GET /api/files/download?transfer_id=...`

## 7. WebSocket

Endpoint:

- `GET /ws?token=<JWT>&device_id=<DEVICE_ID>`

Eventos principais enviados pelo servidor:

- `session_request`
- `session_response`
- `session_started`
- `session_signal`
- `file_available`
- `error`

Evento enviado pelo cliente:

- `session_signal` (relay de frame/input e outros sinais de sessão)

## 8. Fluxo Completo do MVP (Teste Manual)

1. Inicie backend e dois clientes (duas máquinas ou duas instâncias).
2. Cadastre/logue o mesmo usuário nas duas instâncias.
3. Cada cliente registra um dispositivo em `/api/devices/register`.
4. No dashboard, escolha o dispositivo alvo e clique em `Conectar`.
5. No alvo, aceite a solicitação no modal.
6. O solicitante inicia sessão (`/api/sessions/start` acionado pelo app).
7. No host, clique em `Iniciar compartilhamento (host)` e selecione a tela.
8. No controller, visualize frames remotos e envie mouse/teclado pela área de visualização.
9. Envie arquivo pelo bloco de transferência e baixe no receptor.
10. Consulte logs de conexão na UI e auditoria no banco (`audit_logs`).

## 9. Segurança Implementada

- Hash de senha com bcrypt.
- JWT assinado (HS256) com expiração.
- Token temporário por sessão remota.
- Validação de autorização por usuário/dispositivo/sessão.
- Aceite explícito obrigatório pelo dispositivo alvo (modo assistido).
- Rate limit básico no login por IP.
- CORS configurável por `.env`.
- Logs de auditoria mínimos em `audit_logs`.

## 10. Deploy no Render (Free)

Arquivo pronto: `render.yaml` (raiz).

Passos:

1. Suba o repositório no GitHub.
2. No Render, crie `New +` -> `Blueprint` e selecione o repo.
3. O Render lerá `render.yaml`.
4. Configure/valide variáveis de ambiente (especialmente `JWT_SECRET` e `CORS_ALLOWED_ORIGINS`).
5. Deploy.

Observação:

- Para produção robusta, migrar de SQLite para Postgres gerenciado.
- A camada já está preparada via `DB_DRIVER`/`DB_URL` para evolução futura.

## 11. Build do Executável Desktop

```powershell
cd client
npm install
npm run build
```

O Tauri irá gerar instaladores/binários em `client/src-tauri/target`.

## 12. Banco de Dados

Tabela mínima atendida:

- `users`
- `devices`
- `session_requests`
- `remote_sessions`
- `file_transfers`
- `audit_logs`

Migrations em `server/migrations/001_init.sql`.

## 13. Limitações Conhecidas (MVP)

- Relay de sessão simplificado no servidor (sem P2P avançado).
- Injeção nativa de mouse/teclado no host está com base preparada e TODO explícito em Rust (`apply_input_event`).
- Não há refresh endpoint dedicado (token de refresh já é emitido no login para fase seguinte).

## 14. Próxima Fase Recomendada

1. Implementar injeção de input nativa por SO com abstração segura.
2. Adicionar endpoint de refresh token + revogação.
3. Criar adapter PostgreSQL real e migração de dados.
4. Otimizar codec/frame pipeline (delta frames, compressão adaptativa).
5. Evoluir para WebRTC/P2P com TURN/STUN.
6. Adicionar CI real em `.github/workflows`.

## 15. Checklist

### Implementado

- [x] Estrutura `/server` e `/client` conforme solicitado.
- [x] Backend Go modular por camadas.
- [x] SQLite com migrations.
- [x] Registro/login, JWT, `/api/me`.
- [x] Registro de dispositivo e status online/offline.
- [x] Solicitação, aceite/rejeição e início de sessão.
- [x] WebSocket `/ws` para sinalização e relay MVP.
- [x] Compartilhamento de tela básico (host) e render no controller.
- [x] Envio de eventos de input (controller -> host, com stub nativo documentado).
- [x] Transferência simples de arquivo com limite por `.env`.
- [x] Logs de auditoria.
- [x] CORS configurável, middleware de log/recover/auth e rate limit de login.
- [x] Dockerfile do servidor.
- [x] `render.yaml` para Render.
- [x] `.env.example` backend e cliente.
- [x] Scripts de dev/build backend e cliente.
- [x] Base de CI futura (`.github/workflows/ci.placeholder.yml`).

### Preparado para próxima fase

- [x] Base de configuração para migração de DB via `DB_DRIVER`/`DB_URL`.
- [x] Fluxo de sessão modular para evolução para P2P.
- [x] Stub documentado para camada nativa de controle remoto por plataforma.
