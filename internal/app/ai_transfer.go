package app

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"preferans/config"
	"strings"
	"time"
)

type aiTransfer struct {
	ID        string
	Encrypted []byte
	Until     time.Time
	Key       []byte
	Failures  int
}
type aiBundle struct {
	Settings AISettings        `json:"settings"`
	Keys     map[string]string `json:"keys"`
}

func transferCode(value string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(value)))
}
func transferID(code string) string {
	h := sha256.Sum256([]byte("Preferans-AI-lookup-v1:" + code))
	return hex.EncodeToString(h[:])
}
func transferCipher(key []byte) cipher.AEAD {
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	return gcm
}

// The short code authorizes one delivery. Encryption uses random keys and
// ephemeral X25519, so captured ciphertext cannot be decrypted by guessing codes.
func (a *App) StartAITransfer() (string, error) {
	if !config.AIEnabled {
		return "", aiError("build-disabled")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.secretCipher == nil {
		return "", aiError("vault-unavailable")
	}
	keys := map[string]string{}
	for _, k := range a.ai.settings.Keys {
		if a.ai.encrypted[k.ID] == "" {
			continue
		}
		value, err := a.secretCipher.Open(a.ai.encrypted[k.ID])
		if err != nil {
			return "", aiError("key-load")
		}
		keys[k.ID] = value
	}
	data, err := json.Marshal(aiBundle{a.ai.settings, keys})
	if err != nil {
		return "", err
	}
	number, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%06d", number.Int64())
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return "", err
	}
	gcm := transferCipher(key)
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	encrypted := gcm.Seal(nonce, nonce, data, []byte("Preferans-AI-transfer-v1"))
	a.ai.transfer = &aiTransfer{ID: transferID(code), Encrypted: encrypted, Until: time.Now().Add(5 * time.Minute), Key: key}
	return code, nil
}
func (a *App) AITransferHandler() http.Handler {
	if !config.AIEnabled {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		transfer := a.ai.transfer
		if transfer == nil || time.Now().After(transfer.Until) {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/ai-transfer/"+transfer.ID {
			transfer.Failures++
			if transfer.Failures >= 10 {
				a.ai.transfer = nil
			}
			http.NotFound(w, r)
			return
		}
		var request struct {
			PublicKey []byte `json:"publicKey"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&request) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		remote, err := ecdh.X25519().NewPublicKey(request.PublicKey)
		if err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		private, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			http.Error(w, "transfer failed", 500)
			return
		}
		shared, err := private.ECDH(remote)
		if err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		stored := transferCipher(transfer.Key)
		data, err := stored.Open(nil, transfer.Encrypted[:stored.NonceSize()], transfer.Encrypted[stored.NonceSize():], []byte("Preferans-AI-transfer-v1"))
		if err != nil {
			http.Error(w, "transfer failed", 500)
			return
		}
		derived := sha256.Sum256(append([]byte("Preferans-AI-X25519-v1:"), shared...))
		gcm := transferCipher(derived[:])
		nonce := make([]byte, gcm.NonceSize())
		if _, err = rand.Read(nonce); err != nil {
			http.Error(w, "transfer failed", 500)
			return
		}
		encrypted := gcm.Seal(nonce, nonce, data, []byte("Preferans-AI-transfer-v1"))
		_ = json.NewEncoder(w).Encode(map[string][]byte{"encrypted": encrypted, "publicKey": private.PublicKey().Bytes()})
		a.ai.transfer = nil
	})
}
func (a *App) ImportAITransfer(address, code string) error {
	if !config.AIEnabled {
		return aiError("build-disabled")
	}
	code = transferCode(code)
	if len(code) != 6 {
		return aiError("transfer-code")
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return aiError("transfer-code")
		}
	}
	u, err := url.Parse(strings.TrimRight(address, "/"))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return aiError("invalid-url")
	}
	a.mu.Lock()
	available := a.secretCipher != nil
	a.mu.Unlock()
	if !available {
		return aiError("vault-unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	private, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return aiError("transfer-failed")
	}
	body, _ := json.Marshal(map[string][]byte{"publicKey": private.PublicKey().Bytes()})
	req, err := http.NewRequestWithContext(ctx, "POST", u.String()+"/ai-transfer/"+transferID(code), bytes.NewReader(body))
	if err != nil {
		return aiError("transfer-failed")
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return aiError("transfer-failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return aiError("transfer-failed")
	}
	var wire struct {
		Encrypted []byte `json:"encrypted"`
		PublicKey []byte `json:"publicKey"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&wire) != nil {
		return aiError("transfer-failed")
	}
	remote, err := ecdh.X25519().NewPublicKey(wire.PublicKey)
	if err != nil {
		return aiError("transfer-failed")
	}
	shared, err := private.ECDH(remote)
	if err != nil {
		return aiError("transfer-failed")
	}
	derived := sha256.Sum256(append([]byte("Preferans-AI-X25519-v1:"), shared...))
	gcm := transferCipher(derived[:])
	if len(wire.Encrypted) < gcm.NonceSize() {
		return aiError("transfer-failed")
	}
	data, err := gcm.Open(nil, wire.Encrypted[:gcm.NonceSize()], wire.Encrypted[gcm.NonceSize():], []byte("Preferans-AI-transfer-v1"))
	if err != nil {
		return aiError("transfer-failed")
	}
	var bundle aiBundle
	if json.Unmarshal(data, &bundle) != nil {
		return aiError("transfer-failed")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if bundle.Settings.TimeoutSeconds < 1 || bundle.Settings.TimeoutSeconds > 120 {
		return aiError("timeout")
	}
	aliases, ids := map[string]bool{}, map[string]bool{}
	for _, m := range bundle.Settings.Models {
		if m.Validate() != nil || aliases[strings.ToLower(m.Alias)] || ids[m.ID] {
			return aiError("invalid-model")
		}
		aliases[strings.ToLower(m.Alias)] = true
		ids[m.ID] = true
	}
	sealed := map[string]string{}
	keys := map[string]bool{}
	for _, k := range bundle.Settings.Keys {
		if k.ID == "" || k.Name == "" || keys[k.ID] || len(bundle.Keys[k.ID]) == 0 || len(bundle.Keys[k.ID]) > 4096 {
			return aiError("key-invalid")
		}
		keys[k.ID] = true
		value, e := a.secretCipher.Seal(bundle.Keys[k.ID])
		if e != nil {
			return aiError("key-save")
		}
		sealed[k.ID] = value
	}
	for _, m := range bundle.Settings.Models {
		if m.KeyID != "" && !keys[m.KeyID] {
			return aiError("key-missing")
		}
	}
	if err = a.config.Save("ai-keys", sealed); err != nil {
		return err
	}
	if err = a.config.Save("ai-settings", bundle.Settings); err != nil {
		_ = a.config.Save("ai-keys", a.ai.encrypted)
		return err
	}
	a.ai.encrypted = sealed
	a.ai.settings = bundle.Settings
	a.cancelAILocked()
	if a.saved.State != nil {
		s := a.saved.State.Clone()
		if a.updateBotNamesLocked(s) {
			s.Revision++
			sv := a.saved
			sv.State = s
			if err = a.store.Save(s.ID, sv); err != nil {
				return err
			}
			a.saved = sv
			a.broadcastLocked()
		}
	}
	return nil
}
