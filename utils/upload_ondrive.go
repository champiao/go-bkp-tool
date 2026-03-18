package ondrive_upload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	chunkSize = 4 * 1024 * 1024 // 4MB por chunk
	maxSimple = 4 * 1024 * 1024 // acima de 4MB usa upload em sessão
)

// ── Upload simples (< 4MB) ───────────────────────────────────────────────────

func uploadSimple(token, user, folder, localFile string, data []byte) error {
	filename := filepath.Base(localFile)
	uploadURL := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/users/%s/drive/root:%s/%s:/content",
		user, folder, filename,
	)

	req, _ := http.NewRequest("PUT", uploadURL, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("upload simples retornou %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ── Upload em sessão/chunks (>= 4MB) ─────────────────────────────────────────

type uploadSession struct {
	UploadURL string `json:"uploadUrl"`
}

func uploadChunked(token, user, folder, localFile string, data []byte) error {
	filename := filepath.Base(localFile)

	// 1. Cria sessão de upload
	sessionURL := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/users/%s/drive/root:%s/%s:/createUploadSession",
		user, folder, filename,
	)

	sessionBody := map[string]interface{}{
		"item": map[string]string{
			"@microsoft.graph.conflictBehavior": "replace",
			"name":                              filename,
		},
	}
	sessionJSON, _ := json.Marshal(sessionBody)

	req, _ := http.NewRequest("POST", sessionURL, bytes.NewReader(sessionJSON))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("criar sessão: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("criar sessão retornou %d: %s", resp.StatusCode, string(body))
	}

	var session uploadSession
	json.Unmarshal(body, &session)
	fmt.Printf("   📤 Sessão criada, enviando em chunks de 4MB...\n")

	// 2. Envia os chunks
	totalSize := len(data)
	offset := 0

	for offset < totalSize {
		end := offset + chunkSize
		if end > totalSize {
			end = totalSize
		}

		chunk := data[offset:end]
		contentRange := fmt.Sprintf("bytes %d-%d/%d", offset, end-1, totalSize)

		chunkReq, _ := http.NewRequest("PUT", session.UploadURL, bytes.NewReader(chunk))
		chunkReq.Header.Set("Content-Range", contentRange)
		chunkReq.Header.Set("Content-Type", "application/octet-stream")

		chunkResp, err := (&http.Client{}).Do(chunkReq)
		if err != nil {
			return fmt.Errorf("enviar chunk %d: %w", offset, err)
		}
		chunkBody, _ := io.ReadAll(chunkResp.Body)
		chunkResp.Body.Close()

		if chunkResp.StatusCode != 202 && chunkResp.StatusCode != 201 && chunkResp.StatusCode != 200 {
			return fmt.Errorf("chunk %d retornou %d: %s", offset, chunkResp.StatusCode, string(chunkBody))
		}

		offset = end
		fmt.Printf("   ⬆️  %.1f / %.1f MB\n",
			float64(offset)/1024/1024,
			float64(totalSize)/1024/1024,
		)
	}

	return nil
}

// ── Upload (decide simples ou chunked) ───────────────────────────────────────

func UploadFile(token, user, folder, localFile string) error {
	data, err := os.ReadFile(localFile)
	if err != nil {
		return fmt.Errorf("ler arquivo: %w", err)
	}

	size := len(data)
	filename := filepath.Base(localFile)
	fmt.Printf("📁 Enviando: %s (%.2f MB)\n", filename, float64(size)/1024/1024)

	if size < maxSimple {
		fmt.Println("   Modo: upload simples")
		return uploadSimple(token, user, folder, localFile, data)
	}

	fmt.Println("   Modo: upload em chunks (arquivo > 4MB)")
	return uploadChunked(token, user, folder, localFile, data)
}
