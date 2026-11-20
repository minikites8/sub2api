package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const codexAccountIdentityNamespaceVersion = "v1"

const codexAccountIdentitySourceContextKey = "openai_codex_account_identity_source"

// prepareCodexAccountIdentitySource resolves credential shadows once per selected
// attempt. The handler reuses gin.Context across failover attempts, so every entry
// point overwrites the staged source before projecting outbound identity.
func (s *OpenAIGatewayService) prepareCodexAccountIdentitySource(ctx context.Context, c *gin.Context, account *Account) (*Account, error) {
	source := account
	if account != nil && account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return nil, err
		}
		source = resolved
	}
	if c != nil {
		c.Set(codexAccountIdentitySourceContextKey, source)
	}
	return source, nil
}

func codexAccountIdentitySource(c *gin.Context, fallback *Account) *Account {
	if c != nil {
		if staged, ok := c.Get(codexAccountIdentitySourceContextKey); ok {
			if source, ok := staged.(*Account); ok && source != nil {
				return source
			}
		}
	}
	return fallback
}

// codexAccountIdentityNamespace returns a stable, credential-scoped namespace.
// Multiple local rows that use the same ChatGPT account intentionally share the
// same namespace. Setup tokens use an irreversible bearer fingerprint because
// they have no refresh lifecycle or imported account metadata. Refreshable OAuth
// otherwise falls back only to a persistent fingerprint seed: local row IDs are
// deployment-relative and must never become upstream identity.
func codexAccountIdentityNamespace(account *Account) string {
	if account != nil && account.AuthConnectionID != "" {
		return "connection:" + account.AuthConnectionID
	}
	if account == nil || !account.IsOpenAIOAuthLike() {
		return ""
	}
	if upstreamAccountID := strings.TrimSpace(account.GetChatGPTAccountID()); upstreamAccountID != "" {
		if upstreamUserID := strings.TrimSpace(account.GetCredential("chatgpt_user_id")); upstreamUserID != "" {
			return "chatgpt:" + upstreamAccountID + ":user:" + upstreamUserID
		}
		return "chatgpt:" + upstreamAccountID
	}
	if seed, ok := codexFingerprintSeed(account.Extra); ok {
		return "seed:" + seed
	}
	if account.Type == AccountTypeSetupToken {
		if token := strings.TrimSpace(account.GetOpenAIAccessToken()); token != "" {
			sum := sha256.Sum256([]byte("openai-setup-token:" + token))
			return fmt.Sprintf("setup-token:%x", sum[:16])
		}
	}
	return ""
}

// isolateOpenAIUpstreamSessionID preserves the existing API-key isolation while
// adding the selected OAuth credential namespace. A scheduler failover therefore
// cannot send the same session/conversation identity through two upstream accounts.
func isolateOpenAIUpstreamSessionID(apiKeyID int64, account *Account, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	namespace := codexAccountIdentityNamespace(account)
	if namespace == "" {
		return isolateOpenAISessionID(apiKeyID, raw)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("u%d:a%s:%s", apiKeyID, namespace, raw)))
	return fmt.Sprintf("%x", sum[:8])
}

func scopeCodexAccountIdentityValue(account *Account, apiKeyID int64, kind, raw string) string {
	raw = strings.TrimSpace(raw)
	namespace := codexAccountIdentityNamespace(account)
	if raw == "" || namespace == "" {
		return raw
	}
	return deriveStableUUIDv4(fmt.Sprintf(
		"sub2api:codex-account-identity:%s:user:%d:account:%s:kind:%s:value:%s",
		codexAccountIdentityNamespaceVersion,
		apiKeyID,
		namespace,
		kind,
		raw,
	))
}

type codexIdentityFields []struct {
	name string
	kind string
}

var codexAccountIdentityFields = codexIdentityFields{
	{name: "installation_id", kind: "installation"},
	{name: "x-codex-installation-id", kind: "installation"},
	{name: "session_id", kind: "session"},
	{name: "session-id", kind: "session"},
	{name: "thread_id", kind: "thread"},
	{name: "thread-id", kind: "thread"},
	{name: "turn_id", kind: "turn"},
	{name: "turn-id", kind: "turn"},
	{name: "window_id", kind: "window"},
	{name: "x-codex-window-id", kind: "window"},
	{name: "x-client-request-id", kind: "request"},
}

// Off preserves client turn IDs. Parent/root references must map in the
// same domain as the thread they point to, not as independent identifiers.
var codexOffIdentityFields = func() codexIdentityFields {
	var fields codexIdentityFields
	for _, field := range codexAccountIdentityFields {
		if field.kind != "turn" {
			fields = append(fields, field)
		}
	}
	return append(fields, codexIdentityFields{{"parent_thread_id", "thread"}, {"root_thread_id", "thread"}, {codexParentThreadIDHeader, "thread"}}...)
}()

type codexIdentityProjection bool

const (
	legacyCodexProjection codexIdentityProjection = false
	offCodexProjection    codexIdentityProjection = true
)

func codexResponsesProjection(account *Account, native bool) codexIdentityProjection {
	return codexIdentityProjection(native && codexOffResponses(account))
}
func (p codexIdentityProjection) fields() codexIdentityFields {
	if p == offCodexProjection {
		return codexOffIdentityFields
	}
	return codexAccountIdentityFields
}
func (p codexIdentityProjection) enabled(account *Account) bool {
	return (p == offCodexProjection && account != nil) || codexAccountIdentityNamespace(account) != ""
}
func (p codexIdentityProjection) scope(account *Account, key int64, kind, raw string) string {
	if p == offCodexProjection && codexAccountIdentityNamespace(account) == "" {
		// No durable credential namespace. Use the same key-local fallback on every carrier.
		return deriveStableUUIDv4(fmt.Sprintf("sub2api:codex-key-local:v1:%d:%s:%s", key, kind, strings.TrimSpace(raw)))
	}
	return scopeCodexAccountIdentityValue(account, key, kind, raw)
}

func codexOffResponses(account *Account) bool {
	return account.UsesOpenAICodexProtocol() && account.GetCodexFingerprintMode() == codexFingerprintOff
}

// Project only explicit logical identifiers. Never use prompt_cache_key to
// infer a session or conversation. applyHeaders scopes these copied values once.
func copyCodexOffSessionHeaders(dst http.Header, c *gin.Context, source *Account, apiKeyID int64) {
	dst.Del("session_id")
	dst.Del("conversation_id")
	if c == nil || c.Request == nil {
		return
	}
	in := c.Request.Header
	if turn := in.Get("x-client-turn-id"); strings.TrimSpace(turn) != "" {
		dst.Set("x-client-turn-id", turn)
	}
	for _, name := range []string{"session-id", "thread-id", codexParentThreadIDHeader} {
		raw := strings.TrimSpace(in.Get(name))
		if name == "session-id" {
			raw = extractClientSessionID(in)
		}
		if raw == "" {
			continue
		}
		dst.Set(name, raw)
	}
	if raw := strings.TrimSpace(in.Get("conversation_id")); raw != "" {
		dst.Set("conversation_id", isolateOpenAIUpstreamSessionID(apiKeyID, source, raw))
	}
}

func (p codexIdentityProjection) applyFields(values map[string]any, account *Account, apiKeyID int64) bool {
	if values == nil || !p.enabled(account) {
		return false
	}
	changed := false
	for _, field := range p.fields() {
		raw, ok := values[field.name].(string)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		next := p.scope(account, apiKeyID, field.kind, raw)
		if next != raw {
			values[field.name] = next
			changed = true
		}
	}
	return changed
}

func (p codexIdentityProjection) applyEmbedded(values map[string]any, account *Account, apiKeyID int64) bool {
	raw, ok := values[openAIWSTurnMetadataHeader].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return false
	}
	metadata := map[string]any{}
	if err := decodeOpenAIJSONUseNumber([]byte(raw), &metadata); err != nil || metadata == nil {
		return false
	}
	if !p.applyFields(metadata, account, apiKeyID) {
		return false
	}
	rebuilt, err := marshalCodexTurnMetadata(metadata)
	if err != nil {
		return false
	}
	values[openAIWSTurnMetadataHeader] = string(rebuilt)
	return true
}

// Apply both accepted Responses metadata locations from their original values.
// Do not flatten the response envelope or replace unrelated nested fields.
func applyCodexAccountIdentityClientMetadataMap(requestBody map[string]any, account *Account, apiKeyID int64) bool {
	return legacyCodexProjection.applyMap(requestBody, account, apiKeyID)
}

func (p codexIdentityProjection) applyMap(requestBody map[string]any, account *Account, apiKeyID int64) bool {
	if requestBody == nil || !p.enabled(account) {
		return false
	}
	changed := p.applySection(requestBody, account, apiKeyID)
	if response, ok := requestBody["response"].(map[string]any); ok {
		if p.applySection(response, account, apiKeyID) {
			changed = true
		}
	}
	return changed
}

func (p codexIdentityProjection) applySection(requestBody map[string]any, account *Account, apiKeyID int64) bool {
	if requestBody == nil || !p.enabled(account) {
		return false
	}
	changed := false
	clientMetadata, _ := requestBody["client_metadata"].(map[string]any)
	originalBodySessionID := ""
	if clientMetadata != nil {
		originalBodySessionID, _ = clientMetadata["session_id"].(string)
		if p.applyFields(clientMetadata, account, apiKeyID) {
			changed = true
		}
		if p.applyEmbedded(clientMetadata, account, apiKeyID) {
			changed = true
		}
	}
	if raw, ok := requestBody["prompt_cache_key"].(string); ok && strings.TrimSpace(raw) != "" {
		kind := "prompt-cache"
		if p == legacyCodexProjection && strings.TrimSpace(originalBodySessionID) != "" && raw == originalBodySessionID {
			kind = "session"
		}
		next := p.scope(account, apiKeyID, kind, raw)
		if next != raw {
			requestBody["prompt_cache_key"] = next
			changed = true
		}
	}
	return changed
}

// applyCodexAccountIdentityClientMetadataRaw scopes only the small identity
// subobjects with gjson/sjson. The passthrough hot path never unmarshals the
// potentially multi-megabyte request body.
func applyCodexAccountIdentityClientMetadataRaw(body []byte, account *Account, apiKeyID int64) ([]byte, bool, error) {
	return legacyCodexProjection.applyRaw(body, account, apiKeyID)
}

func (p codexIdentityProjection) applyRaw(body []byte, account *Account, apiKeyID int64) ([]byte, bool, error) {
	if len(body) == 0 || !p.enabled(account) {
		return body, false, nil
	}
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return body, false, nil
	}

	next := body
	changed := false
	for _, prefix := range []string{"", "response."} {
		originalBodySessionID := ""
		if cm := root.Get(prefix + "client_metadata"); cm.IsObject() {
			clientMetadata := map[string]any{}
			if err := decodeOpenAIJSONUseNumber([]byte(cm.Raw), &clientMetadata); err != nil {
				return body, false, fmt.Errorf("decode client_metadata for account identity: %w", err)
			}
			originalBodySessionID, _ = clientMetadata["session_id"].(string)
			metadataChanged := p.applyFields(clientMetadata, account, apiKeyID)
			if p.applyEmbedded(clientMetadata, account, apiKeyID) {
				metadataChanged = true
			}
			if metadataChanged {
				raw, err := json.Marshal(clientMetadata)
				if err != nil {
					return body, false, fmt.Errorf("encode account-scoped client_metadata: %w", err)
				}
				var setErr error
				next, setErr = sjson.SetRawBytes(next, prefix+"client_metadata", raw)
				if setErr != nil {
					return body, false, fmt.Errorf("splice account-scoped client_metadata: %w", setErr)
				}
				changed = true
			}
		}
		if promptCacheKey := root.Get(prefix + "prompt_cache_key"); promptCacheKey.Type == gjson.String && strings.TrimSpace(promptCacheKey.String()) != "" {
			raw := promptCacheKey.String()
			kind := "prompt-cache"
			if p == legacyCodexProjection && strings.TrimSpace(originalBodySessionID) != "" && raw == originalBodySessionID {
				kind = "session"
			}
			scoped := p.scope(account, apiKeyID, kind, raw)
			if scoped != raw {
				rewritten, err := sjson.SetBytes(next, prefix+"prompt_cache_key", scoped)
				if err != nil {
					return body, false, fmt.Errorf("splice account-scoped prompt_cache_key: %w", err)
				}
				next = rewritten
				changed = true
			}
		}
	}
	return next, changed, nil
}

func applyCodexAccountIdentityHeaders(headers http.Header, account *Account, apiKeyID int64) {
	legacyCodexProjection.applyHeaders(headers, account, apiKeyID)
}

func (p codexIdentityProjection) applyHeaders(headers http.Header, account *Account, apiKeyID int64) {
	if headers == nil || !p.enabled(account) {
		return
	}
	for _, field := range p.fields() {
		// Underscore session/conversation headers are rebuilt separately from the
		// prompt cache key by each request builder.
		if field.name == "session_id" {
			continue
		}
		raw := strings.TrimSpace(headers.Get(field.name))
		if raw != "" {
			headers.Set(field.name, p.scope(account, apiKeyID, field.kind, raw))
		}
	}
	if raw := strings.TrimSpace(headers.Get(openAIWSTurnMetadataHeader)); raw != "" {
		metadata := map[string]any{}
		if err := decodeOpenAIJSONUseNumber([]byte(raw), &metadata); err == nil && metadata != nil && p.applyFields(metadata, account, apiKeyID) {
			if rebuilt, err := marshalCodexTurnMetadata(metadata); err == nil {
				headers.Set(openAIWSTurnMetadataHeader, string(rebuilt))
			}
		}
	}
}
