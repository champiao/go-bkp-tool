# supabase-bkp-bot :rocket:

Ferramenta em Go para realizar backups automáticos de um banco PostgreSQL no Supabase e enviá-los para o OneDrive via Microsoft Graph API.

## Como funciona

1.  Executa `pg_dump` contra o banco Supabase usando conexão direta via SSL
2.  Salva o dump em `./bkps/backup_YYYY-MM-DD_HH-MM-SS.sql`
3.  Obtém um access token da Microsoft via OAuth2 (`client_credentials`)
4.  Faz upload do arquivo para o OneDrive (simples para arquivos < 4MB, chunked para maiores)
5.  Remove o arquivo local após o upload

## Pré-requisitos

-   Go 1.21+
-   `pg_dump` instalado (`apt install postgresql-client`)
-   App registrado no Azure com permissão `Files.ReadWrite.All` (application permission) no Microsoft Graph

## Configuração

Crie um arquivo `.env` na raiz do projeto:

```env
# Supabase / PostgreSQL
SUPABASE_DB_HOST=aws-1-sa-east-1.pooler.supabase.com
SUPABASE_DB_PORT=5432
SUPABASE_DB_NAME=postgres
SUPABASE_DB_USER=postgres.PROJECT_REF
SUPABASE_DB_PASSWORD=sua_senha_aqui
# Azure / Microsoft GraphMS_CLIENT_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
MS_CLIENT_SECRET=seu_client_secret
MS_TENANT_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
# OneDrive
ONEDRIVE_USER=usuario@empresa.com
ONEDRIVE_FOLDER=/backups/supabase
```

## Instalação e uso


```bash
# Clonar e instalar dependências
git clone https://github.com/champiao/supabase-bkp-bot
cd supabase-bkp-bot
go work init
go work use .
go mod tidy
# Criar pasta de backups temporários
mkdir -p bkps
# Executar
go run ./cmd/main.go
# Ou compilar e executar
go build -o bkp-bot ./cmd/main.go
./bkp-bot
```

## Configurando o app no Azure

1.  Acesse o [Portal Azure](https://portal.azure.com) → **Azure Active Directory** → **App registrations** → **New registration**
2.  Anote o **Application (client) ID** e o **Directory (tenant) ID**
3.  Em **Certificates & secrets**, crie um novo client secret e anote o valor
4.  Em **API permissions**, adicione `Microsoft Graph` → `Files.ReadWrite.All` (Application) e conceda o admin consent

## Agendamento (cron)

Para executar o backup diariamente às 2h:

```cron
0 2 * * * /caminho/para/bkp-bot >> /var/log/supabase-bkp.log 2>&1
```

## Estrutura do projeto

```
.├── cmd/│
   └── main.go              # Entrada principal: pg_dump + token + upload
   ├── utils/│   
   └── upload_ondrive.go    # Upload para OneDrive (simples e chunked)
├── bkps/                    # Dumps temporários (ignorado pelo git)
├── go.mod
└── .env                     # Variáveis de ambiente (ignorado pelo git)
```