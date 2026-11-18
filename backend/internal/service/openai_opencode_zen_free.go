package service

import (
	"container/list"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openCodeZenSessionIDHeader = "X-OpenCode-Session-Id"
	openCodeZenRequestHeader   = "X-OpenCode-Request"
	openCodeZenSessionLimit    = 4096
	openCodeZenSessionTTL      = 24 * time.Hour
)

// Keep the free-client adaptation on official Zen inference endpoints. A free
// model on Go, a paid model, OAuth, or a third-party relay retains its existing
// protocol, authentication and session semantics.
func isOpenCodeZenFreeRequest(account *Account, targetURL string, body []byte) bool {
	if account == nil || account.Type != AccountTypeAPIKey || !isOfficialOpenCodeHost(targetURL) {
		return false
	}
	parsed, err := url.Parse(targetURL)
	if err != nil || (parsed.Path != "/zen/v1/chat/completions" && parsed.Path != "/zen/v1/responses") {
		return false
	}
	model := strings.TrimPrefix(strings.TrimSpace(gjson.GetBytes(body, "model").String()), "opencode/")
	return strings.HasSuffix(model, "-free") || model == "big-pickle"
}

// Missing tools are needed on tool-less health probes and compaction requests.
// tool_choice=none prevents the model from calling compatibility placeholders
// that the downstream client cannot execute. Real tool definitions and choices
// are kept verbatim instead of introducing additional callable tools.
func normalizeOpenCodeFreeTierBody(account *Account, targetURL string, body []byte) []byte {
	if !isOpenCodeZenFreeRequest(account, targetURL, body) || !gjson.ValidBytes(body) {
		return body
	}
	updated, err := sjson.SetBytes(body, "stream", true)
	if err != nil {
		return body
	}
	parsed, _ := url.Parse(targetURL)
	if parsed.Path == "/zen/v1/chat/completions" {
		updated, err = sjson.SetBytes(updated, "stream_options.include_usage", true)
		if err != nil {
			return body
		}
	}
	updated = ensureOpenCodeZenEnvironment(updated, parsed.Path == "/zen/v1/responses")
	if tools := gjson.GetBytes(body, "tools"); tools.Exists() && tools.Type != gjson.Null && (!tools.IsArray() || len(tools.Array()) > 0) {
		return updated
	}
	if choice := gjson.GetBytes(body, "tool_choice"); choice.Exists() && choice.Type != gjson.Null && choice.String() != "none" && choice.String() != "auto" {
		return updated
	}
	responses := parsed.Path == "/zen/v1/responses"
	tools := make([]any, 0, 2)
	for _, name := range []string{"bash", "read"} {
		definition := map[string]any{
			"name":        name,
			"description": "Compatibility placeholder; disabled by tool_choice=none.",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		}
		if responses {
			definition["type"] = "function"
			tools = append(tools, definition)
		} else {
			tools = append(tools, map[string]any{"type": "function", "function": definition})
		}
	}
	updated, err = sjson.SetBytes(updated, "tools", tools)
	if err == nil {
		updated, err = sjson.SetBytes(updated, "tool_choice", "none")
	}
	if err != nil {
		return body
	}
	return updated
}

// The bounded LRU maps client session strings to freshly minted OpenCode IDs.
// It stores only digests, isolates local API keys/accounts, and keeps a stable
// session across turns and protocol conversions without an unbounded map.
type openCodeZenSessionEntry struct {
	key      [32]byte
	id       string
	lastUsed time.Time
}

type openCodeZenSessionCache struct {
	sync.Mutex
	entries map[[32]byte]*list.Element
	order   *list.List
}

var openCodeZenSessions = openCodeZenSessionCache{
	entries: make(map[[32]byte]*list.Element),
	order:   list.New(),
}

func (cache *openCodeZenSessionCache) resolve(key [32]byte) string {
	cache.Lock()
	defer cache.Unlock()
	now := time.Now()
	if element := cache.entries[key]; element != nil {
		entry := element.Value.(*openCodeZenSessionEntry)
		if now.Sub(entry.lastUsed) < openCodeZenSessionTTL {
			entry.lastUsed = now
			cache.order.MoveToFront(element)
			return entry.id
		}
		cache.order.Remove(element)
		delete(cache.entries, key)
	}
	for cache.order.Len() >= openCodeZenSessionLimit {
		oldest := cache.order.Back()
		delete(cache.entries, oldest.Value.(*openCodeZenSessionEntry).key)
		cache.order.Remove(oldest)
	}
	entry := &openCodeZenSessionEntry{key: key, id: newOpenCodeZenID("ses_", true), lastUsed: now}
	cache.entries[key] = cache.order.PushFront(entry)
	return entry.id
}

var openCodeZenIDSequence struct {
	sync.Mutex
	millis  int64
	counter uint64
}

// Matches packages/schema/src/identifier.ts in the official OpenCode client:
// a 48-bit timestamp/counter and 14 random base62 characters. Session IDs sort
// descending, while message IDs sort ascending.
func newOpenCodeZenID(prefix string, descending bool) string {
	openCodeZenIDSequence.Lock()
	now := time.Now().UnixMilli()
	if now != openCodeZenIDSequence.millis {
		openCodeZenIDSequence.millis = now
		openCodeZenIDSequence.counter = 0
	}
	openCodeZenIDSequence.counter++
	value := uint64(now)*0x1000 + openCodeZenIDSequence.counter
	openCodeZenIDSequence.Unlock()
	if descending {
		value = ^value
	}
	var tail [14]byte
	_, _ = rand.Read(tail[:]) // crypto/rand.Read cannot fail on supported Go versions.
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	for i, b := range tail {
		tail[i] = alphabet[int(b)%len(alphabet)]
	}
	return fmt.Sprintf("%s%012x%s", prefix, value&0xffffffffffff, tail[:])
}

func isOpenCodeZenID(value, prefix string) bool {
	if len(value) != len(prefix)+26 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, char := range value[len(prefix) : len(prefix)+12] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	for _, char := range value[len(prefix)+12:] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
			return false
		}
	}
	return true
}

func applyOpenCodeZenFreeHeaders(c *gin.Context, account *Account, headers http.Header, bodies ...[]byte) {
	seed := ""
	if c != nil && c.Request != nil {
		seed = sanitizeSessionID(c.GetHeader(openCodeZenSessionIDHeader))
	}
	if seed == "" {
		seed = resolveOpenCodeSessionID(c, headers, false, bodies...)
	}
	session := seed
	if !isOpenCodeZenID(session, "ses_") {
		if seed == "" {
			// No conversation identity: pin only within this request, including
			// retries, rather than sharing a session across unrelated callers.
			contextKey := "opencode_zen_free_session:" + strconv.FormatInt(account.ID, 10)
			if c != nil {
				seed = c.GetString(contextKey)
			}
			if seed == "" {
				seed = newOpenCodeZenID("ses_", true)
				if c != nil {
					c.Set(contextKey, seed)
				}
			}
		}
		key := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d\x00%s", account.ID, getAPIKeyIDFromContext(c), seed)))
		session = openCodeZenSessions.resolve(key)
	}
	for key := range headers {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "x-stainless-") || strings.HasPrefix(lower, "x-codex-") || lower == "originator" || lower == "openai-beta" {
			delete(headers, key)
		}
	}
	for _, header := range []string{openCodeSessionHeader, openCodeZenSessionIDHeader, openCodeSessionAffinityHeader, openCodeSessionIDHeader} {
		setOpenCodeZenHeader(headers, header, session)
	}
	requestID := openCodeZenHeader(headers, openCodeZenRequestHeader)
	if !isOpenCodeZenID(requestID, "msg_") {
		requestID = newOpenCodeZenID("msg_", false)
	}
	setOpenCodeZenHeader(headers, openCodeZenRequestHeader, requestID)
	for header, fallback := range map[string]string{"X-OpenCode-Client": "cli", "X-OpenCode-Project": "global"} {
		value := openCodeZenHeader(headers, header)
		if value == "" {
			value = fallback
		}
		setOpenCodeZenHeader(headers, header, value)
	}
	headers.Set("Accept", "text/event-stream")
}

func openCodeZenHeader(headers http.Header, name string) string {
	if value := headers.Get(name); value != "" {
		return value
	}
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func setOpenCodeZenHeader(headers http.Header, name, value string) {
	for key := range headers {
		if strings.EqualFold(key, name) {
			delete(headers, key)
		}
	}
	headers.Set(name, value)
}

func openCodeZenTLSProfile(req *http.Request, account *Account, configured *tlsfingerprint.Profile) *tlsfingerprint.Profile {
	if configured != nil || account == nil || account.Type != AccountTypeAPIKey || req == nil || req.URL == nil || req.Method != http.MethodPost || req.GetBody == nil {
		return configured
	}
	if !isOfficialOpenCodeHost(req.URL.String()) || (req.URL.Path != "/zen/v1/chat/completions" && req.URL.Path != "/zen/v1/responses") {
		return configured
	}
	reader, err := req.GetBody()
	if err != nil {
		return configured
	}
	defer func() { _ = reader.Close() }()
	var body struct {
		Model string `json:"model"`
	}
	if json.NewDecoder(reader).Decode(&body) != nil {
		return configured
	}
	modelBody, _ := json.Marshal(body)
	if !isOpenCodeZenFreeRequest(account, req.URL.String(), modelBody) {
		return configured
	}
	return tlsfingerprint.OpenCodeBunProfile()
}
