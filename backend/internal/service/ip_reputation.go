package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ippureAPIBaseURL = "https://api.123169.xyz"
	ippureReferer    = "https://ippure.com/"
	ippureUserAgent  = "Mozilla/5.0 (compatible; Sub2API-AntiAbuse/1.0; +https://ippure.com/)"
)

type cachedIPReputation struct {
	score     int
	expiresAt time.Time
}
type ippureSessionState struct {
	sync.RWMutex
	key          string
	timeOffsetMS int64
}

var (
	ipReputationCache sync.Map
	ippureSession     ippureSessionState
)

// LookupConfiguredIPReputation queries a configured provider or IPPure's
// observed upstream data sources. Provider failures preserve local scoring.
func (s *SettingService) LookupConfiguredIPReputation(ctx context.Context, ip string) int {
	endpoint, apiKey := s.GetAntiAbuseIPReputationConfig(ctx)
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return 0
	}
	provider := endpoint
	if provider == "" {
		provider = "ippure"
	}
	cacheKey := provider + "|" + ip
	if cached, ok := ipReputationCache.Load(cacheKey); ok {
		entry := cached.(cachedIPReputation)
		if time.Now().Before(entry.expiresAt) {
			return entry.score
		}
		ipReputationCache.Delete(cacheKey)
	}

	requestCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	score := 0
	if endpoint == "" || strings.EqualFold(endpoint, "ippure") || strings.EqualFold(endpoint, "ippure://upstream") {
		score = lookupIPPureReputation(requestCtx, http.DefaultClient, ip)
	} else {
		score = lookupGenericIPReputation(requestCtx, http.DefaultClient, endpoint, apiKey, ip)
	}
	if score > 0 {
		ipReputationCache.Store(cacheKey, cachedIPReputation{score: score, expiresAt: time.Now().Add(15 * time.Minute)})
	}
	return score
}

func lookupIPPureReputation(ctx context.Context, client *http.Client, ip string) int {
	var risk struct {
		OK   bool `json:"ok"`
		Data struct {
			RiskScore float64 `json:"risk_score"`
		} `json:"data"`
	}
	if err := ippureGetJSON(ctx, client, "/api/info/ip-risk/"+url.PathEscape(ip), &risk); err != nil || !risk.OK {
		return 0
	}
	score := clampRiskScore(int(risk.Data.RiskScore))

	var basic struct {
		OK   bool `json:"ok"`
		Data struct {
			ASN struct {
				Number        int64  `json:"number"`
				Type          string `json:"type"`
				IsResidential bool   `json:"is_residential"`
			} `json:"asn"`
		} `json:"data"`
	}
	if err := ippureGetJSON(ctx, client, "/api/info/ip-basic/"+url.PathEscape(ip), &basic); err == nil && basic.OK {
		if !basic.Data.ASN.IsResidential && (strings.EqualFold(basic.Data.ASN.Type, "hosting") || strings.EqualFold(basic.Data.ASN.Type, "business")) {
			score = maxInt(score, 15)
		}
		if basic.Data.ASN.Number > 0 {
			var traffic struct {
				OK   bool `json:"ok"`
				Data struct {
					Bot   float64 `json:"bot"`
					Human float64 `json:"human"`
				} `json:"data"`
			}
			if err := ippureGetJSON(ctx, client, fmt.Sprintf("/api/info/asn/botclass/%d", basic.Data.ASN.Number), &traffic); err == nil && traffic.OK {
				switch {
				case traffic.Data.Bot >= 90:
					score = maxInt(score, 20)
				case traffic.Data.Bot >= 70:
					score = maxInt(score, 12)
				case traffic.Data.Bot >= 50:
					score = maxInt(score, 6)
				}
			}
		}
	}
	if privacyScore := lookupIPInfoPrivacy(ctx, client, ip); privacyScore > score {
		score = privacyScore
	}
	return clampRiskScore(score)
}

func ippureGetJSON(ctx context.Context, client *http.Client, path string, target any) error {
	requestURL := ippureAPIBaseURL + path
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Referer", ippureReferer)
		req.Header.Set("Origin", strings.TrimRight(ippureReferer, "/"))
		req.Header.Set("User-Agent", ippureUserAgent)
		ippureSession.RLock()
		key, offset := ippureSession.key, ippureSession.timeOffsetMS
		ippureSession.RUnlock()
		if key != "" {
			timestamp := time.Now().UnixMilli() + offset
			req.Header.Set("x-k", key)
			req.Header.Set("x-t", ippureSignature(http.MethodGet, requestURL, "", timestamp, key))
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		if nextKey := strings.TrimSpace(resp.Header.Get("x-k")); nextKey != "" {
			serverTime, _ := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("x-t")), 10, 64)
			ippureSession.Lock()
			ippureSession.key = nextKey
			if serverTime > 0 {
				ippureSession.timeOffsetMS = serverTime - time.Now().UnixMilli()
			}
			ippureSession.Unlock()
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && json.Unmarshal(body, target) == nil {
			return nil
		}
	}
	return fmt.Errorf("ippure upstream response unavailable")
}

func ippureSignature(method, requestURL, body string, timestamp int64, key string) string {
	payload := strings.Join([]string{method, requestURL, body, strconv.FormatInt(timestamp, 10)}, "-")
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(payload))
	return strconv.FormatInt(timestamp, 10) + "-" + hex.EncodeToString(mac.Sum(nil))
}

func lookupIPInfoPrivacy(ctx context.Context, client *http.Client, ip string) int {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipinfo.io/widget/demo/"+url.PathEscape(ip), nil)
	if err != nil {
		return 0
	}
	req.Header.Set("Referer", ippureReferer)
	req.Header.Set("User-Agent", ippureUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0
	}
	var payload struct {
		Data struct {
			Privacy struct {
				VPN     bool `json:"vpn"`
				Proxy   bool `json:"proxy"`
				Tor     bool `json:"tor"`
				Relay   bool `json:"relay"`
				Hosting bool `json:"hosting"`
			} `json:"privacy"`
			IsHosting   bool `json:"is_hosting"`
			IsAnonymous bool `json:"is_anonymous"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload) != nil {
		return 0
	}
	score := 0
	if payload.Data.Privacy.VPN {
		score = maxInt(score, 25)
	}
	if payload.Data.Privacy.Proxy {
		score = maxInt(score, 35)
	}
	if payload.Data.Privacy.Tor {
		score = maxInt(score, 45)
	}
	if payload.Data.Privacy.Relay {
		score = maxInt(score, 20)
	}
	if payload.Data.Privacy.Hosting || payload.Data.IsHosting {
		score = maxInt(score, 15)
	}
	if payload.Data.IsAnonymous {
		score = maxInt(score, 30)
	}
	return score
}

func lookupGenericIPReputation(ctx context.Context, client *http.Client, endpoint, apiKey, ip string) int {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return 0
	}
	query := parsed.Query()
	query.Set("ip", ip)
	parsed.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return 0
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0
	}
	var payload any
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload) != nil {
		return 0
	}
	return findRiskScore(payload)
}

func findRiskScore(payload any) int {
	if object, ok := payload.(map[string]any); ok {
		for _, key := range []string{"score", "risk_score", "reputation"} {
			if score, exists := object[key]; exists {
				if parsed, valid := numberToInt(score); valid {
					return clampRiskScore(parsed)
				}
			}
		}
		for _, value := range object {
			if score := findRiskScore(value); score > 0 {
				return score
			}
		}
	}
	return 0
}

func clampRiskScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func numberToInt(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := strconv.Atoi(string(typed))
		return parsed, err == nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}
