package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	oauthstorage "github.com/giantswarm/mcp-oauth/storage"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

type storedToken struct {
	AccessToken  string         `json:"access_token,omitempty"`
	TokenType    string         `json:"token_type,omitempty"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	Expiry       time.Time      `json:"expiry,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path cannot be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Stop() {
	_ = s.db.Close()
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`CREATE TABLE IF NOT EXISTS oauth_tokens (
			user_id TEXT PRIMARY KEY,
			token_json BLOB NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_user_infos (
			user_id TEXT PRIMARY KEY,
			info_json BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_refresh_tokens (
			refresh_token TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_clients (
			client_id TEXT PRIMARY KEY,
			client_json BLOB NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_auth_states (
			state_id TEXT PRIMARY KEY,
			provider_state TEXT NOT NULL UNIQUE,
			state_json BLOB NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_auth_states_provider_state ON oauth_auth_states(provider_state)`,
		`CREATE TABLE IF NOT EXISTS oauth_auth_codes (
			code TEXT PRIMARY KEY,
			code_json BLOB NOT NULL,
			expires_at INTEGER NOT NULL,
			used INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_token_metadata (
			token_id TEXT PRIMARY KEY,
			metadata_json BLOB NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) SaveToken(ctx context.Context, userID string, token *oauth2.Token) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}
	if token == nil {
		return fmt.Errorf("token cannot be nil")
	}
	data, err := json.Marshal(tokenToStored(token))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO oauth_tokens(user_id, token_json, expires_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET token_json = excluded.token_json, expires_at = excluded.expires_at`,
		userID, data, unix(token.Expiry))
	return err
}

func (s *SQLiteStore) GetToken(ctx context.Context, userID string) (*oauth2.Token, error) {
	var data []byte
	if err := s.db.QueryRowContext(ctx, `SELECT token_json FROM oauth_tokens WHERE user_id = ?`, userID).Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", oauthstorage.ErrTokenNotFound, userID)
		}
		return nil, err
	}
	token, err := tokenFromJSON(data)
	if err != nil {
		return nil, err
	}
	if !token.Expiry.IsZero() && token.Expiry.Before(time.Now()) && token.RefreshToken == "" {
		return nil, fmt.Errorf("%w: %s", oauthstorage.ErrTokenExpired, userID)
	}
	return token, nil
}

func (s *SQLiteStore) DeleteToken(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_tokens WHERE user_id = ?`, userID)
	return err
}

func (s *SQLiteStore) SaveUserInfo(ctx context.Context, userID string, info *oauthstorage.UserInfo) error {
	if userID == "" || info == nil {
		return fmt.Errorf("invalid user info")
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO oauth_user_infos(user_id, info_json)
		VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET info_json = excluded.info_json`, userID, data)
	return err
}

func (s *SQLiteStore) GetUserInfo(ctx context.Context, userID string) (*oauthstorage.UserInfo, error) {
	var data []byte
	if err := s.db.QueryRowContext(ctx, `SELECT info_json FROM oauth_user_infos WHERE user_id = ?`, userID).Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", oauthstorage.ErrUserInfoNotFound, userID)
		}
		return nil, err
	}
	var info oauthstorage.UserInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (s *SQLiteStore) SaveRefreshToken(ctx context.Context, refreshToken, userID string, expiresAt time.Time) error {
	if refreshToken == "" || userID == "" {
		return fmt.Errorf("refresh token and userID cannot be empty")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_refresh_tokens(refresh_token, user_id, expires_at)
		VALUES (?, ?, ?)
		ON CONFLICT(refresh_token) DO UPDATE SET user_id = excluded.user_id, expires_at = excluded.expires_at`,
		refreshToken, userID, unix(expiresAt))
	return err
}

func (s *SQLiteStore) GetRefreshTokenInfo(ctx context.Context, refreshToken string) (string, error) {
	var userID string
	var expiresAt int64
	if err := s.db.QueryRowContext(ctx, `SELECT user_id, expires_at FROM oauth_refresh_tokens WHERE refresh_token = ?`, refreshToken).Scan(&userID, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: refresh token not found", oauthstorage.ErrTokenNotFound)
		}
		return "", err
	}
	if expired(expiresAt) {
		return "", fmt.Errorf("%w: refresh token expired", oauthstorage.ErrTokenExpired)
	}
	return userID, nil
}

func (s *SQLiteStore) DeleteRefreshToken(ctx context.Context, refreshToken string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_refresh_tokens WHERE refresh_token = ?`, refreshToken)
	return err
}

func (s *SQLiteStore) AtomicGetAndDeleteRefreshToken(ctx context.Context, refreshToken string) (string, string, *oauth2.Token, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", nil, err
	}
	defer tx.Rollback()

	var userID string
	var expiresAt int64
	if err := tx.QueryRowContext(ctx, `SELECT user_id, expires_at FROM oauth_refresh_tokens WHERE refresh_token = ?`, refreshToken).Scan(&userID, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", nil, fmt.Errorf("%w: %s", oauthstorage.ErrTokenNotFound, oauthstorage.ErrMsgRefreshTokenNotFoundOrUsed)
		}
		return "", "", nil, err
	}
	if expired(expiresAt) {
		return "", "", nil, fmt.Errorf("%w: refresh token expired", oauthstorage.ErrTokenExpired)
	}

	var tokenData []byte
	if err := tx.QueryRowContext(ctx, `SELECT token_json FROM oauth_tokens WHERE user_id = ?`, userID).Scan(&tokenData); err != nil {
		return "", "", nil, fmt.Errorf("%w: provider token not found", oauthstorage.ErrTokenNotFound)
	}
	providerToken, err := tokenFromJSON(tokenData)
	if err != nil {
		return "", "", nil, err
	}

	clientID := ""
	if meta, err := s.getTokenMetadataTx(ctx, tx, refreshToken); err == nil && meta != nil {
		clientID = meta.ClientID
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_refresh_tokens WHERE refresh_token = ?`, refreshToken); err != nil {
		return "", "", nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_token_metadata WHERE token_id = ?`, refreshToken); err != nil {
		return "", "", nil, err
	}
	if err := tx.Commit(); err != nil {
		return "", "", nil, err
	}
	return userID, clientID, providerToken, nil
}

func (s *SQLiteStore) SaveClient(ctx context.Context, client *oauthstorage.Client) error {
	if client == nil || client.ClientID == "" {
		return fmt.Errorf("invalid client")
	}
	data, err := json.Marshal(client)
	if err != nil {
		return err
	}
	created := client.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO oauth_clients(client_id, client_json, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(client_id) DO UPDATE SET client_json = excluded.client_json`,
		client.ClientID, data, unix(created))
	return err
}

func (s *SQLiteStore) GetClient(ctx context.Context, clientID string) (*oauthstorage.Client, error) {
	var data []byte
	if err := s.db.QueryRowContext(ctx, `SELECT client_json FROM oauth_clients WHERE client_id = ?`, clientID).Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", oauthstorage.ErrClientNotFound, clientID)
		}
		return nil, err
	}
	var client oauthstorage.Client
	if err := json.Unmarshal(data, &client); err != nil {
		return nil, err
	}
	return &client, nil
}

func (s *SQLiteStore) ValidateClientSecret(ctx context.Context, clientID, clientSecret string) error {
	client, err := s.GetClient(ctx, clientID)
	hash := oauthstorage.DummyBcryptHash
	public := false
	if err == nil {
		public = client.IsPublic()
		if client.ClientSecretHash != "" {
			hash = client.ClientSecretHash
		}
	}
	bcryptErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(clientSecret))
	if public && err == nil {
		return nil
	}
	if err != nil || bcryptErr != nil {
		return fmt.Errorf("invalid client credentials")
	}
	return nil
}

func (s *SQLiteStore) ListClients(ctx context.Context) ([]*oauthstorage.Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT client_json FROM oauth_clients ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var clients []*oauthstorage.Client
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var client oauthstorage.Client
		if err := json.Unmarshal(data, &client); err != nil {
			return nil, err
		}
		clients = append(clients, &client)
	}
	return clients, rows.Err()
}

func (s *SQLiteStore) DeleteClient(ctx context.Context, clientID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_clients WHERE client_id = ?`, clientID)
	return err
}

func (s *SQLiteStore) CheckIPLimit(context.Context, string, int) error {
	return nil
}

func (s *SQLiteStore) SaveAuthorizationState(ctx context.Context, state *oauthstorage.AuthorizationState) error {
	if state == nil || state.StateID == "" {
		return fmt.Errorf(oauthstorage.ErrMsgInvalidAuthorizationState)
	}
	if state.ProviderState == "" {
		return fmt.Errorf(oauthstorage.ErrMsgProviderStateRequired)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO oauth_auth_states(state_id, provider_state, state_json, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(state_id) DO UPDATE SET provider_state = excluded.provider_state, state_json = excluded.state_json, expires_at = excluded.expires_at`,
		state.StateID, state.ProviderState, data, unix(state.ExpiresAt))
	return err
}

func (s *SQLiteStore) GetAuthorizationState(ctx context.Context, stateID string) (*oauthstorage.AuthorizationState, error) {
	return s.getAuthorizationState(ctx, `SELECT state_json, expires_at FROM oauth_auth_states WHERE state_id = ?`, stateID)
}

func (s *SQLiteStore) GetAuthorizationStateByProviderState(ctx context.Context, providerState string) (*oauthstorage.AuthorizationState, error) {
	return s.getAuthorizationState(ctx, `SELECT state_json, expires_at FROM oauth_auth_states WHERE provider_state = ?`, providerState)
}

func (s *SQLiteStore) DeleteAuthorizationState(ctx context.Context, stateID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_auth_states WHERE state_id = ? OR provider_state = ?`, stateID, stateID)
	return err
}

func (s *SQLiteStore) SaveAuthorizationCode(ctx context.Context, code *oauthstorage.AuthorizationCode) error {
	if code == nil || code.Code == "" {
		return fmt.Errorf("invalid authorization code")
	}
	data, err := json.Marshal(code)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO oauth_auth_codes(code, code_json, expires_at, used)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(code) DO UPDATE SET code_json = excluded.code_json, expires_at = excluded.expires_at, used = excluded.used`,
		code.Code, data, unix(code.ExpiresAt), boolInt(code.Used))
	return err
}

func (s *SQLiteStore) GetAuthorizationCode(ctx context.Context, code string) (*oauthstorage.AuthorizationCode, error) {
	var data []byte
	var expiresAt int64
	if err := s.db.QueryRowContext(ctx, `SELECT code_json, expires_at FROM oauth_auth_codes WHERE code = ?`, code).Scan(&data, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, oauthstorage.ErrAuthorizationCodeNotFound
		}
		return nil, err
	}
	if expired(expiresAt) {
		return nil, fmt.Errorf("%w: authorization code expired", oauthstorage.ErrTokenExpired)
	}
	return authCodeFromJSON(data)
}

func (s *SQLiteStore) AtomicCheckAndMarkAuthCodeUsed(ctx context.Context, code string) (*oauthstorage.AuthorizationCode, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var data []byte
	var expiresAt int64
	var used int
	if err := tx.QueryRowContext(ctx, `SELECT code_json, expires_at, used FROM oauth_auth_codes WHERE code = ?`, code).Scan(&data, &expiresAt, &used); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, oauthstorage.ErrAuthorizationCodeNotFound
		}
		return nil, err
	}
	authCode, err := authCodeFromJSON(data)
	if err != nil {
		return nil, err
	}
	if expired(expiresAt) {
		return nil, fmt.Errorf("%w: authorization code expired", oauthstorage.ErrTokenExpired)
	}
	if used != 0 || authCode.Used {
		return authCode, oauthstorage.ErrAuthorizationCodeUsed
	}
	authCode.Used = true
	updated, err := json.Marshal(authCode)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE oauth_auth_codes SET used = 1, code_json = ? WHERE code = ?`, updated, code); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return authCode, nil
}

func (s *SQLiteStore) DeleteAuthorizationCode(ctx context.Context, code string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_auth_codes WHERE code = ?`, code)
	return err
}

func (s *SQLiteStore) SaveTokenMetadata(ctx context.Context, tokenID string, metadata oauthstorage.TokenMetadata) error {
	if tokenID == "" || metadata.UserID == "" || metadata.ClientID == "" {
		return fmt.Errorf("tokenID, userID, and clientID cannot be empty")
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO oauth_token_metadata(token_id, metadata_json, expires_at)
		VALUES (?, ?, ?)
		ON CONFLICT(token_id) DO UPDATE SET metadata_json = excluded.metadata_json, expires_at = excluded.expires_at`,
		tokenID, data, unix(metadata.ExpiresAt))
	return err
}

func (s *SQLiteStore) GetTokenMetadata(tokenID string) (*oauthstorage.TokenMetadata, error) {
	return s.getTokenMetadata(context.Background(), tokenID)
}

func (s *SQLiteStore) getAuthorizationState(ctx context.Context, query, arg string) (*oauthstorage.AuthorizationState, error) {
	var data []byte
	var expiresAt int64
	if err := s.db.QueryRowContext(ctx, query, arg).Scan(&data, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", oauthstorage.ErrAuthorizationStateNotFound, arg)
		}
		return nil, err
	}
	if expired(expiresAt) {
		return nil, fmt.Errorf("%w: authorization state expired", oauthstorage.ErrTokenExpired)
	}
	var state oauthstorage.AuthorizationState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *SQLiteStore) getTokenMetadata(ctx context.Context, tokenID string) (*oauthstorage.TokenMetadata, error) {
	var data []byte
	if err := s.db.QueryRowContext(ctx, `SELECT metadata_json FROM oauth_token_metadata WHERE token_id = ?`, tokenID).Scan(&data); err != nil {
		return nil, err
	}
	var meta oauthstorage.TokenMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (s *SQLiteStore) getTokenMetadataTx(ctx context.Context, tx *sql.Tx, tokenID string) (*oauthstorage.TokenMetadata, error) {
	var data []byte
	if err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM oauth_token_metadata WHERE token_id = ?`, tokenID).Scan(&data); err != nil {
		return nil, err
	}
	var meta oauthstorage.TokenMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func tokenToStored(token *oauth2.Token) storedToken {
	return storedToken{
		AccessToken:  token.AccessToken,
		TokenType:    token.TokenType,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
		Extra:        oauthstorage.ExtractTokenExtra(token),
	}
}

func tokenFromJSON(data []byte) (*oauth2.Token, error) {
	var stored storedToken
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	token := &oauth2.Token{
		AccessToken:  stored.AccessToken,
		TokenType:    stored.TokenType,
		RefreshToken: stored.RefreshToken,
		Expiry:       stored.Expiry,
	}
	if len(stored.Extra) > 0 {
		token = token.WithExtra(stored.Extra)
	}
	return token, nil
}

func authCodeFromJSON(data []byte) (*oauthstorage.AuthorizationCode, error) {
	var code oauthstorage.AuthorizationCode
	if err := json.Unmarshal(data, &code); err != nil {
		return nil, err
	}
	return &code, nil
}

func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func expired(ts int64) bool {
	return ts > 0 && time.Unix(ts, 0).Before(time.Now())
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
