package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"time"

	utils "github.com/champiao/supabase-bkp-bot/utils"
	"github.com/joho/godotenv"
)

// ── Variáveis de ambiente necessárias ───────────────────────────────────────
// SUPABASE_DB_HOST     → Host do pooler     ex: aws-1-sa-east-1.pooler.supabase.com
// SUPABASE_DB_PORT     → Porta              ex: 5432
// SUPABASE_DB_NAME     → Nome do banco      ex: postgres
// SUPABASE_DB_USER     → Usuário do pooler  ex: postgres.PROJECT_REF
// SUPABASE_DB_PASSWORD → Senha do banco     (suporta qualquer caractere especial)
// MS_CLIENT_ID         → Application (client) ID do app Azure
// MS_CLIENT_SECRET     → Client Secret do app Azure
// MS_TENANT_ID         → Directory (tenant) ID do Azure
// ONEDRIVE_USER        → Email/UPN do usuário dono do OneDrive (ex: user@empresa.com)
// ONEDRIVE_FOLDER      → Caminho da pasta no OneDrive (ex: /backups/supabase)
// ────────────────────────────────────────────────────────────────────────────

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "❌  Variável de ambiente ausente: %s\n", key)
		os.Exit(1)
	}
	return v
}

// ── Cria arquivo .pgpass temporário ─────────────────────────────────────────
// Evita qualquer problema de caracteres especiais na senha,
// pois o .pgpass é lido como arquivo, não interpretado pelo shell.

func createPgPass(host, port, dbname, user, password string) (string, error) {
	pgpassPath := "/tmp/.pgpass_backup"

	// formato: host:port:dbname:user:password
	content := fmt.Sprintf("%s:%s:%s:%s:%s\n", host, port, dbname, user, password)

	if err := os.WriteFile(pgpassPath, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("criar .pgpass: %w", err)
	}

	return pgpassPath, nil
}

// ── 1. Backup via pg_dump (conexão direta PostgreSQL) ────────────────────────

func downloadBackup(host, port, dbname, user, password string) (string, error) {
	// Verifica se pg_dump está instalado
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return "", fmt.Errorf("pg_dump não encontrado. Instale com: apt install postgresql-client")
	}

	// Cria .pgpass temporário para evitar problemas com caracteres especiais na senha
	pgpassPath, err := createPgPass(host, port, dbname, user, password)
	if err != nil {
		return "", err
	}
	defer os.Remove(pgpassPath) // remove o .pgpass ao finalizar

	filename := fmt.Sprintf("./bkps/backup_%s.sql", time.Now().Format("2006-01-02_15-04-05"))

	fmt.Println("⏳  Executando pg_dump...")
	// Monta a connection string com sslmode=require explícito
	// Isso evita problemas de SSL com o session pooler do Supabase
	connStr := fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s sslmode=require",
		host, port, dbname, user,
	)

	cmd := exec.Command("pg_dump",
		"--no-owner",
		"--no-acl",
		"--format=plain",
		"--file="+filename,
		"--dbname="+connStr,
	)

	// Aponta para o .pgpass temporário (sem PGPASSWORD, sem conflito de shell)
	cmd.Env = append(os.Environ(), "PGPASSFILE="+pgpassPath)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pg_dump falhou: %w", err)
	}

	// Verifica se o arquivo foi criado e tem conteúdo
	info, err := os.Stat(filename)
	if err != nil {
		return "", fmt.Errorf("arquivo de backup não encontrado após pg_dump: %w", err)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("arquivo de backup está vazio")
	}

	fmt.Printf("✅  Backup salvo em: %s (%.2f MB)\n", filename, float64(info.Size())/1024/1024)
	return filename, nil
}

// ── 2. Obter access token via client_credentials (app-only) ─────────────────

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func getAccessToken(tenantID, clientID, clientSecret string) (string, error) {
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("client_secret", clientSecret)
	params.Set("scope", "https://graph.microsoft.com/.default")
	params.Set("grant_type", "client_credentials")

	resp, err := http.PostForm(tokenURL, params)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("microsoft retornou %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}

	fmt.Println("✅  Access token obtido com sucesso.")
	return tr.AccessToken, nil
}

// ── main ─────────────────────────────────────────────────────────────────────

func main() {
	godotenv.Load()

	dbHost := mustEnv("SUPABASE_DB_HOST")
	dbPort := mustEnv("SUPABASE_DB_PORT")
	dbName := mustEnv("SUPABASE_DB_NAME")
	dbUser := mustEnv("SUPABASE_DB_USER")
	dbPassword := mustEnv("SUPABASE_DB_PASSWORD")

	msClientID := mustEnv("MS_CLIENT_ID")
	msClientSecret := mustEnv("MS_CLIENT_SECRET")
	msTenantID := mustEnv("MS_TENANT_ID")
	onedriveUser := mustEnv("ONEDRIVE_USER")
	onedriveFolder := mustEnv("ONEDRIVE_FOLDER")

	fmt.Println("🔄  Iniciando backup Supabase → OneDrive")
	fmt.Println("─────────────────────────────────────────")

	// 1. Gera o backup via pg_dump usando .pgpass temporário
	backupFile, err := downloadBackup(dbHost, dbPort, dbName, dbUser, dbPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌  Erro no backup: %v\n", err)
		os.Exit(1)
	}

	// 2. Obtém access token via client_credentials
	accessToken, err := getAccessToken(msTenantID, msClientID, msClientSecret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌  Erro ao obter token: %v\n", err)
		os.Exit(1)
	}

	// 3. Faz upload para o OneDrive
	if err := utils.UploadFile(accessToken, onedriveUser, onedriveFolder, backupFile); err != nil {
		fmt.Fprintf(os.Stderr, "❌  Erro no upload: %v\n", err)
		os.Exit(1)
	}

	// 4. Remove o arquivo local
	if err := os.Remove(backupFile); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️   Não foi possível deletar o arquivo local: %v\n", err)
	} else {
		fmt.Printf("🗑️   Arquivo local removido: %s\n", backupFile)
	}

	fmt.Println("─────────────────────────────────────────")
	fmt.Println("🎉  Backup concluído com sucesso!")
}
