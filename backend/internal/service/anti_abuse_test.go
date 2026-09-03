package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBrowserFingerprintsSupportMultipleAndFuzzyMatches(t *testing.T) {
	values := NormalizeBrowserFingerprints([]string{`["os=windows;chrome=136;screen=1920x1080","canvas=abc"]`, "canvas=abc|canvas=abc"})
	require.Len(t, values, 2)
	stored := HashBrowserFingerprints([]string{"os=windows;chrome=136;screen=1920x1080"})
	require.True(t, BrowserFingerprintMatch(stored, []string{"screen=2560x1440;chrome=137;os=windows"}))
	require.False(t, BrowserFingerprintMatch(stored, []string{"os=linux;firefox=120;screen=1280x720"}))
}

func TestIPPureUpstreamHandshakeAndMultiSourceAggregation(t *testing.T) {
	ippureSession.Lock()
	ippureSession.key = ""
	ippureSession.timeOffsetMS = 0
	ippureSession.Unlock()
	calls := make([]string, 0, 6)
	client := &http.Client{Transport: antiAbuseRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.String())
		response := func(body string) *http.Response {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}
		}
		if strings.Contains(req.URL.Path, "/ip-risk/") && req.Header.Get("x-k") == "" {
			resp := response("unauthorized")
			resp.Header.Set("x-k", "session-key")
			resp.Header.Set("x-t", strconv.FormatInt(time.Now().UnixMilli(), 10))
			return resp, nil
		}
		if strings.HasPrefix(req.URL.Host, "api.123169.xyz") {
			require.Equal(t, "session-key", req.Header.Get("x-k"))
			parts := strings.SplitN(req.Header.Get("x-t"), "-", 2)
			require.Len(t, parts, 2)
			timestamp, err := strconv.ParseInt(parts[0], 10, 64)
			require.NoError(t, err)
			require.Equal(t, ippureSignature(http.MethodGet, req.URL.String(), "", timestamp, "session-key"), req.Header.Get("x-t"))
		}
		switch {
		case strings.Contains(req.URL.Path, "/ip-risk/"):
			return response(`{"ok":true,"data":{"risk_score":7}}`), nil
		case strings.Contains(req.URL.Path, "/ip-basic/"):
			return response(`{"ok":true,"data":{"asn":{"number":15169,"type":"hosting","is_residential":false}}}`), nil
		case strings.Contains(req.URL.Path, "/asn/botclass/"):
			return response(`{"ok":true,"data":{"bot":97.06,"human":2.94}}`), nil
		case req.URL.Host == "ipinfo.io":
			return response(`{"data":{"privacy":{"vpn":false,"proxy":false,"tor":false,"relay":false,"hosting":true},"is_hosting":true}}`), nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		}
	})}

	require.Equal(t, 20, lookupIPPureReputation(context.Background(), client, "8.8.8.8"))
	require.Len(t, calls, 5)
}

type antiAbuseRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn antiAbuseRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestEvaluateAntiAbuseCombinesSignals(t *testing.T) {
	assessment := EvaluateAntiAbuse(RiskSignals{
		IPAddress: "203.0.113.10", Email: "temp-user@mailinator.com", UserAgent: "python-requests/2.32", IPReputationScore: 50,
	}, 2, 1, 2, DefaultAntiAbusePolicy())
	require.Equal(t, AntiAbuseActionRestrict, assessment.Action)
	require.GreaterOrEqual(t, assessment.Score, 60)
	require.Contains(t, assessment.Factors, "browser_fingerprint")
	require.Contains(t, assessment.Factors, "ip_reputation")
	require.Contains(t, assessment.Factors, "email_reputation")
	require.Contains(t, assessment.Factors, "calling_user_agent")
}

func TestEvaluateAntiAbuseWithTLSFingerprints(t *testing.T) {
	assessment := EvaluateAntiAbuseWithTLS(RiskSignals{IPAddress: "203.0.113.20", Email: "person@example.com", UserAgent: "Mozilla/5.0", JA3: strings.Repeat("a", 32), JA4: "t13d1714h1_5b57614c22b0_7baf387fc6ff"}, 0, 0, 0, 2, DefaultAntiAbusePolicy())
	require.Equal(t, AntiAbuseActionAllow, assessment.Action)
	require.Equal(t, 40, assessment.Factors["ja3_ja4_velocity"])
	require.NotEmpty(t, HashTransportFingerprint("ja3", strings.Repeat("a", 32)))
	require.NotEmpty(t, HashTransportFingerprint("ja4", "t13d1714h1_5b57614c22b0_7baf387fc6ff"))
}

func TestDisabledAntiAbuseSkipsScoring(t *testing.T) {
	policy := DefaultAntiAbusePolicy()
	policy.Enabled = false
	assessment := EvaluateAntiAbuseWithTLS(RiskSignals{
		IPAddress: "8.8.8.8", Email: "temp-user@mailinator.com", UserAgent: "python-requests/2.32",
		BrowserFingerprints: []string{"os=windows;chrome=136"}, JA3: strings.Repeat("a", 32),
		JA4: "t13d1714h1_5b57614c22b0_7baf387fc6ff",
	}, 4, 3, 2, 2, policy)
	require.Equal(t, AntiAbuseActionAllow, assessment.Action)
	require.Zero(t, assessment.Score)
	require.Empty(t, assessment.Factors)
}

func TestRegistrationIPThresholdIsMultidimensionalFactor(t *testing.T) {
	policy := DefaultAntiAbusePolicy()
	policy.SignupIPRiskControlThreshold = 3
	signals := RiskSignals{IPAddress: "8.8.8.8", Email: "person@example.com", UserAgent: "Mozilla/5.0"}

	below := EvaluateRegistrationAntiAbuse(signals, 1, 0, 0, 0, policy)
	require.Equal(t, AntiAbuseActionAllow, below.Action)
	require.NotContains(t, below.Factors, "signup_ip_threshold")

	hit := EvaluateRegistrationAntiAbuse(signals, 2, 0, 0, 0, policy)
	require.Equal(t, AntiAbuseActionRestrict, hit.Action)
	require.Equal(t, policy.ScoreThreshold, hit.Factors["signup_ip_threshold"])
}

func TestSharedBrowserFingerprintIsImmediateHighRisk(t *testing.T) {
	policy := DefaultAntiAbusePolicy()
	assessment := EvaluateRegistrationAntiAbuse(
		RiskSignals{IPAddress: "8.8.8.8", Email: "person@example.com", UserAgent: "Mozilla/5.0"},
		0, 1, 0, 0, policy,
	)
	require.Equal(t, AntiAbuseActionRestrict, assessment.Action)
	require.Equal(t, policy.ScoreThreshold, assessment.Factors["browser_fingerprint"])
	require.Contains(t, assessment.Reasons, "browser fingerprint is linked to multiple accounts")
}

func TestAPIIPUAThresholdIsMultidimensionalFactor(t *testing.T) {
	policy := DefaultAntiAbusePolicy()
	policy.APIUsageIPUARiskControlThreshold = 4
	signals := RiskSignals{IPAddress: "8.8.8.8", Email: "person@example.com", UserAgent: "Mozilla/5.0"}

	hit := EvaluateGatewayAntiAbuse(signals, 3, 0, 0, 0, policy)
	require.Equal(t, AntiAbuseActionRestrict, hit.Action)
	require.Equal(t, policy.ScoreThreshold, hit.Factors["api_ip_ua_threshold"])
}

func TestEmailVelocityExemptsQQAndGmail(t *testing.T) {
	policy := DefaultAntiAbusePolicy()
	for _, email := range []string{"user@qq.com", " User@GMAIL.COM ", "user@mail.qq.com"} {
		assessment := EvaluateRegistrationAntiAbuse(
			RiskSignals{IPAddress: "8.8.8.8", Email: email, UserAgent: "Mozilla/5.0"},
			0, 0, 10, 0, policy,
		)
		require.NotContains(t, assessment.Factors, "email_velocity", email)
	}

	assessment := EvaluateRegistrationAntiAbuse(
		RiskSignals{IPAddress: "8.8.8.8", Email: "user@example.com", UserAgent: "Mozilla/5.0"},
		0, 0, 10, 0, policy,
	)
	require.Equal(t, 30, assessment.Factors["email_velocity"])
}

func TestBrowserAccountAttemptsAreNormalizedAndRestricted(t *testing.T) {
	ctx := WithRiskSignals(context.Background(), RiskSignals{AccountAttempts: []string{
		`[" First@Example.com ","second@example.com","bad-value"]`,
		"first@example.com|third@example.com",
	}})
	signals := RiskSignalsFromContext(ctx)
	require.Equal(t, []string{"first@example.com", "second@example.com", "third@example.com"}, signals.AccountAttempts)

	assessment := EvaluateGatewayAntiAbuse(signals, 0, 0, 0, 0, DefaultAntiAbusePolicy())
	require.Equal(t, AntiAbuseActionRestrict, assessment.Action)
	require.Equal(t, defaultAntiAbuseScoreThreshold, assessment.Factors["browser_account_attempts"])

	singleAccount := EvaluateGatewayAntiAbuse(
		RiskSignals{
			IPAddress: "8.8.8.8", Email: "current@example.com", UserAgent: "Mozilla/5.0",
			AccountAttempts: []string{"current@example.com"},
		},
		0, 0, 0, 0, DefaultAntiAbusePolicy(),
	)
	require.NotContains(t, singleAccount.Factors, "browser_account_attempts")
	require.Equal(t, AntiAbuseActionAllow, singleAccount.Action)
}

type recordingAntiAbuseEventStore struct {
	events []AntiAbuseEvent
}

func (s *recordingAntiAbuseEventStore) RecordAntiAbuseEvent(_ context.Context, event *AntiAbuseEvent) error {
	if event != nil {
		s.events = append(s.events, *event)
	}
	return nil
}

func (s *recordingAntiAbuseEventStore) ListAntiAbuseEvents(context.Context, AntiAbuseEventFilter) ([]AntiAbuseEvent, int64, error) {
	return s.events, int64(len(s.events)), nil
}

func TestAllowAndReviewAntiAbuseAssessmentsAreNotRecorded(t *testing.T) {
	store := &recordingAntiAbuseEventStore{}
	clean := AntiAbuseAssessment{Action: AntiAbuseActionAllow, Factors: map[string]int{}, Score: 0}
	RecordAntiAbuseAssessment(context.Background(), store, "gateway", nil, "clean@example.com", RiskSignals{}, clean)
	require.Empty(t, store.events)

	lowRiskAllow := AntiAbuseAssessment{Action: AntiAbuseActionAllow, Factors: map[string]int{"calling_user_agent": 25}, Score: 25}
	RecordAntiAbuseAssessment(context.Background(), store, "gateway", nil, "risk@example.com", RiskSignals{UserAgent: "python-requests/2.32"}, lowRiskAllow)
	require.Empty(t, store.events)

	review := AntiAbuseAssessment{Action: AntiAbuseActionReview, Factors: map[string]int{"ip_reputation": 45}, Score: 45}
	RecordAntiAbuseAssessment(context.Background(), store, "gateway", nil, "review@example.com", RiskSignals{}, review)
	require.Empty(t, store.events)

	restrict := AntiAbuseAssessment{Action: AntiAbuseActionRestrict, Factors: map[string]int{"browser_fingerprint": 60}, Score: 60}
	RecordAntiAbuseAssessment(context.Background(), store, "gateway", nil, "restricted@example.com", RiskSignals{}, restrict)
	require.Len(t, store.events, 1)
	require.Equal(t, 60, store.events[0].Factors["browser_fingerprint"])
}

func TestAntiAbuseConfigIncludesIPVelocityControls(t *testing.T) {
	repo := &antiAbuseSettingRepo{values: map[string]string{
		SettingKeySignupIPRiskControlThreshold:        "7",
		SettingKeySignupIPDisablePreviousAccounts:     "false",
		SettingKeySignupIPKeepPreviousAccounts:        "2",
		SettingKeyAPIUsageIPUARiskControlThreshold:    "8",
		SettingKeyAPIUsageIPUADisablePreviousAccounts: "true",
		SettingKeyAPIUsageIPUAKeepPreviousAccounts:    "3",
	}}
	svc := NewSettingService(repo, &config.Config{})
	view := svc.GetAntiAbuseConfig(context.Background())
	require.Equal(t, 7, view.SignupIPRiskControlThreshold)
	require.False(t, view.SignupIPDisablePreviousAccounts)
	require.Equal(t, 2, view.SignupIPKeepPreviousAccounts)
	require.Equal(t, 8, view.APIUsageIPUARiskControlThreshold)
	require.True(t, view.APIUsageIPUADisablePreviousAccounts)
	require.Equal(t, 3, view.APIUsageIPUAKeepPreviousAccounts)

	updated, err := svc.UpdateAntiAbuseConfig(context.Background(), UpdateAntiAbuseConfigInput{
		Enabled: true, ScoreThreshold: 60, FingerprintWeight: 1, IPWeight: 1, EmailWeight: 1,
		UserAgentWeight: 1, TLSFingerprintWeight: 1, SignupIPRiskControlThreshold: 9,
		SignupIPDisablePreviousAccounts: true, SignupIPKeepPreviousAccounts: 1,
		APIUsageIPUARiskControlThreshold: 10, APIUsageIPUADisablePreviousAccounts: false,
		APIUsageIPUAKeepPreviousAccounts: 0,
	})
	require.NoError(t, err)
	require.Equal(t, 9, updated.SignupIPRiskControlThreshold)
	require.Equal(t, "10", repo.values[SettingKeyAPIUsageIPUARiskControlThreshold])
}

func TestConfiguredIPReputationProviderAndCache(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "203.0.113.9", r.URL.Query().Get("ip"))
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"risk_score":72}`))
	}))
	defer server.Close()
	repo := &antiAbuseSettingRepo{values: map[string]string{SettingKeyAntiAbuseIPReputationEndpoint: server.URL, SettingKeyAntiAbuseIPReputationAPIKey: "secret"}}
	svc := NewSettingService(repo, &config.Config{})
	require.Equal(t, 72, svc.LookupConfiguredIPReputation(context.Background(), "203.0.113.9"))
	require.Equal(t, 72, svc.LookupConfiguredIPReputation(context.Background(), "203.0.113.9"))
	require.Equal(t, 1, calls)
}

type antiAbuseSettingRepo struct{ values map[string]string }

func (r *antiAbuseSettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (r *antiAbuseSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}
func (r *antiAbuseSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}
func (r *antiAbuseSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := map[string]string{}
	for _, key := range keys {
		result[key] = r.values[key]
	}
	return result, nil
}
func (r *antiAbuseSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}
func (r *antiAbuseSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}
func (r *antiAbuseSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}
