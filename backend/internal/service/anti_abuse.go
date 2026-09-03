package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/mail"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	AntiAbuseActionAllow           = "allow"
	AntiAbuseActionReview          = "review"
	AntiAbuseActionRestrict        = "restrict"
	defaultAntiAbuseScoreThreshold = 60
)

var antiAbuseEmailVelocityExemptDomains = map[string]struct{}{
	"gmail.com": {},
	"qq.com":    {},
}

// AntiAbusePolicy controls the score threshold and per-signal contribution.
// Values are integer percentages, which keeps persisted settings portable.
type AntiAbusePolicy struct {
	Enabled                             bool
	ScoreThreshold                      int
	FingerprintWeight                   int
	IPWeight                            int
	EmailWeight                         int
	UserAgentWeight                     int
	TLSFingerprintWeight                int
	SignupIPRiskControlThreshold        int
	SignupIPDisablePreviousAccounts     bool
	SignupIPKeepPreviousAccounts        int
	APIUsageIPUARiskControlThreshold    int
	APIUsageIPUADisablePreviousAccounts bool
	APIUsageIPUAKeepPreviousAccounts    int
}

func DefaultAntiAbusePolicy() AntiAbusePolicy {
	return AntiAbusePolicy{
		Enabled: true, ScoreThreshold: defaultAntiAbuseScoreThreshold,
		FingerprintWeight: 1, IPWeight: 1, EmailWeight: 1, UserAgentWeight: 1, TLSFingerprintWeight: 1,
		SignupIPRiskControlThreshold:        defaultSignupIPRiskControlThreshold,
		SignupIPDisablePreviousAccounts:     defaultSignupIPDisablePreviousAccounts,
		SignupIPKeepPreviousAccounts:        defaultSignupIPKeepPreviousAccounts,
		APIUsageIPUARiskControlThreshold:    defaultAPIUsageIPUARiskControlThreshold,
		APIUsageIPUADisablePreviousAccounts: defaultAPIUsageIPUADisablePreviousAccounts,
		APIUsageIPUAKeepPreviousAccounts:    defaultAPIUsageIPUAKeepPreviousAccounts,
	}
}

type AntiAbuseAssessment struct {
	Score   int            `json:"score"`
	Action  string         `json:"action"`
	Factors map[string]int `json:"factors"`
	Reasons []string       `json:"reasons"`
}

// AntiAbuseEvent is the persisted decision shown in the admin risk-control center.
// Fingerprints and transport identifiers use hashes; account attempts use
// normalized emails so administrators can identify linked users.
type AntiAbuseEvent struct {
	ID                   int64          `json:"id"`
	UserID               *int64         `json:"user_id,omitempty"`
	UserEmail            string         `json:"user_email"`
	EventType            string         `json:"event_type"`
	Action               string         `json:"action"`
	Score                int            `json:"score"`
	Factors              map[string]int `json:"factors"`
	Reasons              []string       `json:"reasons"`
	IPAddress            string         `json:"ip_address"`
	Email                string         `json:"email"`
	UserAgent            string         `json:"user_agent"`
	FingerprintHashCount int            `json:"fingerprint_hash_count"`
	JA3Hash              string         `json:"ja3_hash,omitempty"`
	JA4Hash              string         `json:"ja4_hash,omitempty"`
	AccountAttempts      []string       `json:"account_attempts"`
	GiftBalanceDeducted  float64        `json:"gift_balance_deducted"`
	CreatedAt            time.Time      `json:"created_at"`
}

type AntiAbuseEventFilter struct {
	Pagination     pagination.PaginationParams
	EventType      string
	Action         string
	Search         string
	From           *time.Time
	To             *time.Time
	DeductionsOnly bool
}

type AntiAbuseEventStore interface {
	RecordAntiAbuseEvent(ctx context.Context, event *AntiAbuseEvent) error
	ListAntiAbuseEvents(ctx context.Context, filter AntiAbuseEventFilter) ([]AntiAbuseEvent, int64, error)
}

func (s *ContentModerationService) ListAntiAbuseEvents(ctx context.Context, filter AntiAbuseEventFilter) ([]AntiAbuseEvent, int64, error) {
	if s == nil || s.userRepo == nil {
		return []AntiAbuseEvent{}, 0, nil
	}
	store, ok := s.userRepo.(AntiAbuseEventStore)
	if !ok {
		return []AntiAbuseEvent{}, 0, nil
	}
	return store.ListAntiAbuseEvents(ctx, filter)
}

func RecordAntiAbuseAssessment(ctx context.Context, store AntiAbuseEventStore, eventType string, userID *int64, userEmail string, signals RiskSignals, assessment AntiAbuseAssessment) {
	// The event stream contains only hard restrictions. Allow and review checks
	// remain available to the evaluator without creating risk-center rows.
	if store == nil || assessment.Action != AntiAbuseActionRestrict {
		return
	}
	event := &AntiAbuseEvent{
		UserID: userID, UserEmail: strings.TrimSpace(userEmail), EventType: eventType,
		Action: assessment.Action, Score: assessment.Score, Factors: assessment.Factors,
		Reasons: assessment.Reasons, IPAddress: strings.TrimSpace(signals.IPAddress),
		Email: strings.TrimSpace(signals.Email), UserAgent: strings.TrimSpace(signals.UserAgent),
		FingerprintHashCount: len(HashBrowserFingerprints(signals.BrowserFingerprints)),
		JA3Hash:              HashTransportFingerprint("ja3", signals.JA3), JA4Hash: HashTransportFingerprint("ja4", signals.JA4),
		AccountAttempts: append([]string(nil), signals.AccountAttempts...),
	}
	if err := store.RecordAntiAbuseEvent(ctx, event); err != nil {
		return
	}
}

func RecordAntiAbuseDeduction(ctx context.Context, store AntiAbuseEventStore, eventType string, userID *int64, userEmail string, signals RiskSignals, assessment AntiAbuseAssessment, amount float64) {
	if store == nil || amount <= 0 {
		return
	}
	event := &AntiAbuseEvent{
		UserID: userID, UserEmail: strings.TrimSpace(userEmail), EventType: eventType,
		Action: AntiAbuseActionRestrict, Score: assessment.Score, Factors: assessment.Factors,
		Reasons: assessment.Reasons, IPAddress: strings.TrimSpace(signals.IPAddress),
		Email: strings.TrimSpace(signals.Email), UserAgent: strings.TrimSpace(signals.UserAgent),
		FingerprintHashCount: len(HashBrowserFingerprints(signals.BrowserFingerprints)),
		JA3Hash:              HashTransportFingerprint("ja3", signals.JA3), JA4Hash: HashTransportFingerprint("ja4", signals.JA4),
		AccountAttempts:     append([]string(nil), signals.AccountAttempts...),
		GiftBalanceDeducted: amount,
	}
	_ = store.RecordAntiAbuseEvent(ctx, event)
}

// NormalizeBrowserFingerprints accepts a JSON array, pipe-delimited values, or a single value.
func NormalizeBrowserFingerprints(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	queue := append([]string(nil), values...)
	for len(queue) > 0 {
		raw := queue[0]
		queue = queue[1:]
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "[") {
			var parsed []string
			if json.Unmarshal([]byte(raw), &parsed) == nil {
				queue = append(queue, parsed...)
				continue
			}
		}
		for _, item := range strings.FieldsFunc(raw, func(r rune) bool { return r == '|' || r == '\n' || r == '\r' }) {
			item = strings.TrimSpace(item)
			if item == "" || len(item) > 1024 {
				continue
			}
			canonical := canonicalFingerprint(item)
			if canonical == "" {
				continue
			}
			if _, ok := seen[canonical]; ok {
				continue
			}
			seen[canonical] = struct{}{}
			out = append(out, canonical)
			if len(out) >= 8 {
				return out
			}
		}
	}
	return out
}

func canonicalFingerprint(raw string) string {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(raw)), func(r rune) bool { return r == ';' || r == '&' || r == ',' })
	if len(parts) > 1 {
		sort.Strings(parts)
		return strings.Join(parts, "&")
	}
	return strings.Join(strings.Fields(raw), " ")
}

func HashBrowserFingerprints(values []string) []string {
	values = NormalizeBrowserFingerprints(values)
	result := make([]string, 0, len(values)*2)
	seen := make(map[string]struct{})
	for _, value := range values {
		exact := sha256.Sum256([]byte(value))
		hash := hex.EncodeToString(exact[:])
		coarse := sha256.Sum256([]byte(fuzzyFingerprintBucket(value)))
		bucket := "b:" + hex.EncodeToString(coarse[:8])
		for _, item := range []string{hash, bucket} {
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

// NormalizeAccountAttempts accepts JSON arrays, repeated headers, and
// pipe-delimited values while retaining only valid normalized email accounts.
func NormalizeAccountAttempts(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	queue := append([]string(nil), values...)
	for len(queue) > 0 {
		raw := strings.TrimSpace(queue[0])
		queue = queue[1:]
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "[") {
			var parsed []string
			if json.Unmarshal([]byte(raw), &parsed) == nil {
				queue = append(parsed, queue...)
				continue
			}
		}
		for _, item := range strings.FieldsFunc(raw, func(r rune) bool { return r == '|' || r == '\n' || r == '\r' }) {
			item = strings.ToLower(strings.TrimSpace(item))
			if len(item) == 0 || len(item) > 320 {
				continue
			}
			address, err := mail.ParseAddress(item)
			if err != nil || address.Address != item {
				continue
			}
			if _, exists := seen[item]; exists {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
			if len(out) >= 8 {
				return out
			}
		}
	}
	return out
}

func fuzzyFingerprintBucket(value string) string {
	var builder strings.Builder
	inDigits := false
	for _, char := range strings.ToLower(value) {
		if unicode.IsDigit(char) {
			if !inDigits {
				builder.WriteByte('#')
				inDigits = true
			}
			continue
		}
		inDigits = false
		if unicode.IsSpace(char) {
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func BrowserFingerprintMatch(stored, incoming []string) bool {
	if len(stored) == 0 || len(incoming) == 0 {
		return false
	}
	left := make(map[string]struct{}, len(stored))
	for _, item := range stored {
		left[strings.TrimSpace(item)] = struct{}{}
	}
	for _, item := range HashBrowserFingerprints(incoming) {
		if _, ok := left[item]; ok {
			return true
		}
	}
	return false
}

func EvaluateAntiAbuse(signals RiskSignals, ipVelocity, fingerprintVelocity, emailVelocity int, policy AntiAbusePolicy) AntiAbuseAssessment {
	return evaluateAntiAbuse(signals, ipVelocity, fingerprintVelocity, emailVelocity, 0, policy, 0, "", "")
}

func EvaluateAntiAbuseWithTLS(signals RiskSignals, ipVelocity, fingerprintVelocity, emailVelocity, tlsVelocity int, policy AntiAbusePolicy) AntiAbuseAssessment {
	return evaluateAntiAbuse(signals, ipVelocity, fingerprintVelocity, emailVelocity, tlsVelocity, policy, 0, "", "")
}

// EvaluateRegistrationAntiAbuse applies the registration-IP velocity threshold
// as a hard multidimensional factor.
func EvaluateRegistrationAntiAbuse(signals RiskSignals, ipVelocity, fingerprintVelocity, emailVelocity, tlsVelocity int, policy AntiAbusePolicy) AntiAbuseAssessment {
	threshold := policy.SignupIPRiskControlThreshold
	if threshold < 1 {
		threshold = defaultSignupIPRiskControlThreshold
	}
	return evaluateAntiAbuse(signals, ipVelocity, fingerprintVelocity, emailVelocity, tlsVelocity, policy, threshold, "signup_ip_threshold", "registration IP account threshold reached")
}

// EvaluateGatewayAntiAbuse applies the API IP+UA velocity threshold as a hard
// multidimensional factor.
func EvaluateGatewayAntiAbuse(signals RiskSignals, ipVelocity, fingerprintVelocity, emailVelocity, tlsVelocity int, policy AntiAbusePolicy) AntiAbuseAssessment {
	threshold := policy.APIUsageIPUARiskControlThreshold
	if threshold < 1 {
		threshold = defaultAPIUsageIPUARiskControlThreshold
	}
	return evaluateAntiAbuse(signals, ipVelocity, fingerprintVelocity, emailVelocity, tlsVelocity, policy, threshold, "api_ip_ua_threshold", "API IP and User-Agent account threshold reached")
}

func evaluateAntiAbuse(signals RiskSignals, ipVelocity, fingerprintVelocity, emailVelocity, tlsVelocity int, policy AntiAbusePolicy, velocityThreshold int, velocityFactor, velocityReason string) AntiAbuseAssessment {
	if policy.ScoreThreshold < 1 {
		policy.ScoreThreshold = defaultAntiAbuseScoreThreshold
	}
	if policy.FingerprintWeight < 1 {
		policy.FingerprintWeight = 1
	}
	if policy.IPWeight < 1 {
		policy.IPWeight = 1
	}
	if policy.EmailWeight < 1 {
		policy.EmailWeight = 1
	}
	if policy.UserAgentWeight < 1 {
		policy.UserAgentWeight = 1
	}
	if policy.TLSFingerprintWeight < 1 {
		policy.TLSFingerprintWeight = 1
	}
	if policy.SignupIPRiskControlThreshold < 1 {
		policy.SignupIPRiskControlThreshold = defaultSignupIPRiskControlThreshold
	}
	if policy.APIUsageIPUARiskControlThreshold < 1 {
		policy.APIUsageIPUARiskControlThreshold = defaultAPIUsageIPUARiskControlThreshold
	}
	assessment := AntiAbuseAssessment{Action: AntiAbuseActionAllow, Factors: map[string]int{}}
	if !policy.Enabled {
		return assessment
	}
	add := func(name, reason string, value, weight int) {
		if value <= 0 {
			return
		}
		contribution := value * weight
		assessment.Factors[name] = contribution
		assessment.Score += contribution
		assessment.Reasons = append(assessment.Reasons, reason)
	}

	if fingerprintVelocity > 0 {
		// A browser fingerprint shared by multiple accounts is a hard high-risk
		// association, regardless of the remaining signal score.
		add("browser_fingerprint", "browser fingerprint is linked to multiple accounts", policy.ScoreThreshold, policy.FingerprintWeight)
	}
	if len(signals.AccountAttempts) > 1 {
		add("browser_account_attempts", "browser memory contains multiple account attempts", policy.ScoreThreshold, policy.FingerprintWeight)
	}
	if ipVelocity > 0 {
		value := minInt(ipVelocity*12, 36)
		add("ip_velocity", "IP address shows recent account velocity", value, policy.IPWeight)
	}
	ipReputation := localIPReputationScore(signals.IPAddress)
	if signals.IPReputationScore > ipReputation {
		ipReputation = minInt(signals.IPReputationScore, 100)
	}
	add("ip_reputation", "IP address has a high reputation risk signal", ipReputation, policy.IPWeight)
	if emailVelocity > 0 && !isAntiAbuseEmailVelocityExempt(signals.Email) {
		add("email_velocity", "email domain has recent account velocity", minInt(emailVelocity*10, 30), policy.EmailWeight)
	}
	add("email_reputation", "email address matches a disposable or automation pattern", emailReputationScore(signals.Email), policy.EmailWeight)
	add("calling_user_agent", "calling User-Agent indicates automation or is missing", userAgentReputationScore(signals.UserAgent), policy.UserAgentWeight)
	if tlsVelocity > 0 {
		add("ja3_ja4_velocity", "JA3/JA4 transport fingerprint reused across recent accounts", minInt(20+tlsVelocity*10, 40), policy.TLSFingerprintWeight)
	}
	if tlsScore := transportFingerprintReputationScore(signals.JA3, signals.JA4); tlsScore > 0 {
		add("ja3_ja4_reputation", "JA3/JA4 transport fingerprint is malformed or incomplete", tlsScore, policy.TLSFingerprintWeight)
	}
	if velocityThreshold > 0 && ipVelocity+1 >= velocityThreshold {
		add(velocityFactor, velocityReason, policy.ScoreThreshold, policy.IPWeight)
		assessment.Action = AntiAbuseActionRestrict
	}
	if fingerprintVelocity > 0 || len(signals.AccountAttempts) > 1 {
		assessment.Action = AntiAbuseActionRestrict
	}

	if !policy.Enabled {
		assessment.Action = AntiAbuseActionAllow
	} else if assessment.Score >= policy.ScoreThreshold {
		assessment.Action = AntiAbuseActionRestrict
	} else if assessment.Score*4 >= policy.ScoreThreshold*3 {
		assessment.Action = AntiAbuseActionReview
	}
	return assessment
}

func normalizeTransportFingerprint(raw string, limit int) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if len(raw) > limit {
		raw = raw[:limit]
	}
	return raw
}

func HashTransportFingerprint(kind, value string) string {
	value = normalizeTransportFingerprint(value, 256)
	if value == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(kind)) + ":" + value))
	return hex.EncodeToString(digest[:])
}

func transportFingerprintReputationScore(ja3, ja4 string) int {
	ja3 = normalizeTransportFingerprint(ja3, 128)
	ja4 = normalizeTransportFingerprint(ja4, 256)
	if ja3 == "" && ja4 == "" {
		return 0
	}
	score := 0
	if ja3 != "" && (len(ja3) != 32 || !isLowerHex(ja3)) {
		score = maxInt(score, 15)
	}
	if ja4 != "" && (!strings.Contains(ja4, "_") || len(ja4) < 12) {
		score = maxInt(score, 15)
	}
	if ja3 == "" || ja4 == "" {
		score = maxInt(score, 5)
	}
	return score
}

func isLowerHex(value string) bool {
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func localIPReputationScore(raw string) int {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		if strings.TrimSpace(raw) == "" {
			return 20
		}
		return 35
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return 25
	}
	if ip.IsPrivate() {
		return 15
	}
	return 0
}

func emailReputationScore(raw string) int {
	email := strings.ToLower(strings.TrimSpace(raw))
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" {
		return 25
	}
	disposable := map[string]struct{}{
		"mailinator.com": {}, "10minutemail.com": {}, "guerrillamail.com": {}, "guerrillamail.net": {},
		"temp-mail.org": {}, "tempmail.com": {}, "yopmail.com": {}, "sharklasers.com": {}, "getnada.com": {},
	}
	if _, ok := disposable[domain]; ok {
		return 35
	}
	if strings.Contains(local, "test") || strings.Contains(local, "temp") || strings.Contains(local, "spam") {
		return 12
	}
	return 0
}

func isAntiAbuseEmailVelocityExempt(email string) bool {
	domain := RegistrationEmailDomain(email)
	_, exempt := antiAbuseEmailVelocityExemptDomains[domain]
	return exempt
}

func userAgentReputationScore(raw string) int {
	ua := strings.ToLower(strings.TrimSpace(raw))
	if ua == "" {
		return 20
	}
	for _, marker := range []string{"curl/", "python-requests", "go-http-client", "postmanruntime", "headlesschrome", "selenium", "playwright"} {
		if strings.Contains(ua, marker) {
			return 25
		}
	}
	if !strings.Contains(ua, "mozilla/") && !strings.Contains(ua, "claude-cli") && !strings.Contains(ua, "openai") {
		return 10
	}
	return 0
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
