package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// Evaluation statuses persisted on a decision row. Skipped and dropped
// evaluations are counted in memory only: writing rows while the queue is
// overflowing would add load exactly when the system is under pressure.
const (
	GPTAutoEvaluationSuccess = "success"
	GPTAutoEvaluationFailed  = "failed"
)

// GPTAutoShadowDecision is one persisted shadow evaluation.
type GPTAutoShadowDecision struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`

	RequestID string `json:"request_id"`
	UserID    *int64 `json:"user_id,omitempty"`
	APIKeyID  *int64 `json:"api_key_id,omitempty"`
	GroupID   *int64 `json:"group_id,omitempty"`

	Endpoint     string `json:"endpoint"`
	ActualModel  string `json:"actual_model"`
	ActualEffort string `json:"actual_effort"`
	Stream       bool   `json:"stream"`
	HasTools     bool   `json:"has_tools"`

	EvaluationStatus string `json:"evaluation_status"`
	ErrorClass       string `json:"error_class,omitempty"`
	DecisionStatus   string `json:"decision_status,omitempty"`

	JevModel    string `json:"jev_model"`
	JevEffort   string `json:"jev_effort"`
	FinalModel  string `json:"final_model"`
	FinalEffort string `json:"final_effort"`

	Complexity       *float64 `json:"complexity,omitempty"`
	Category         string   `json:"category"`
	ModelConfidence  *float64 `json:"model_confidence,omitempty"`
	EffortConfidence *float64 `json:"effort_confidence,omitempty"`

	ClassifierModel        string  `json:"classifier_model"`
	ClassifierInputTokens  int     `json:"classifier_input_tokens"`
	ClassifierOutputTokens int     `json:"classifier_output_tokens"`
	ClassifierCost         float64 `json:"classifier_cost"`
	ClassifierLatencyMS    int     `json:"classifier_latency_ms"`
	QueueDelayMS           int     `json:"queue_delay_ms"`
	Attempts               int     `json:"attempts"`

	// Hypothetical economics of the recommendation. EstGrossProfit,
	// EstGrossMargin and MarginBelowTarget are fully loaded (they include the
	// Jev cost), as they always were; the model-only margin is separate.
	// Margins are nil when the estimate had insufficient data.
	EstRevenue             *float64 `json:"est_revenue,omitempty"`
	EstUpstreamCost        *float64 `json:"est_upstream_cost,omitempty"`
	EstGrossProfit         *float64 `json:"est_gross_profit,omitempty"`
	EstGrossMargin         *float64 `json:"est_gross_margin,omitempty"`
	MarginBelowTarget      bool     `json:"margin_below_target"`
	EstModelGrossMargin    *float64 `json:"est_model_gross_margin,omitempty"`
	ModelMarginBelowTarget bool     `json:"model_margin_below_target"`
	// EstimateQuality is empty on rows written before estimate sources existed.
	EstimateQuality string `json:"estimate_quality"`

	PolicyVersion      string `json:"policy_version"`
	PolicySnapshotHash string `json:"policy_snapshot_hash"`
	// ClassifierConfigHash identifies the exact Jev questions (instructions,
	// criteria, model descriptions, state format). Empty on rows classified
	// before versioning, which used the v1 hard-coded questions.
	ClassifierConfigHash string `json:"classifier_config_hash"`

	Features       GPTAutoRequestFeatures `json:"features"`
	Classification *GPTAutoClassification `json:"classification,omitempty"`
	Decision       *GPTAutoPolicyDecision `json:"decision,omitempty"`

	// Attached by GetDecision; never stored.
	//   ActualUsage: the request's usage-log row.
	//   ObservedEconomics: what that request really earned and cost.
	//   ActualInputEconomics: the recommendation re-estimated with the request's
	//     real input tokens and customer rate (output stays an estimate: the
	//     recommended model was never called).
	//   ClassifierConfig: the stored questions this decision was asked with.
	ActualUsage          *GPTAutoActualUsage            `json:"actual_usage,omitempty"`
	ObservedEconomics    *GPTAutoObservedEconomics      `json:"observed_economics,omitempty"`
	ActualInputEconomics *GPTAutoEconomics              `json:"actual_input_economics,omitempty"`
	ClassifierConfig     *GPTAutoClassifierConfigRecord `json:"classifier_config,omitempty"`
}

// GPTAutoPolicySnapshot is the subset of config that determines a policy
// result. Content-addressed so identical configs share one row.
type GPTAutoPolicySnapshot struct {
	PolicyVersion      string                `json:"policy_version"`
	GlobalMinMargin    float64               `json:"global_min_margin"`
	ProfitSafetyBuffer float64               `json:"profit_safety_buffer"`
	ModelProfiles      []GPTAutoModelProfile `json:"model_profiles"`
}

// NewGPTAutoPolicySnapshot extracts the policy-relevant config.
func NewGPTAutoPolicySnapshot(cfg GPTAutoShadowConfig) GPTAutoPolicySnapshot {
	return GPTAutoPolicySnapshot{
		PolicyVersion:      cfg.PolicyVersion,
		GlobalMinMargin:    cfg.GlobalMinMargin,
		ProfitSafetyBuffer: cfg.ProfitSafetyBuffer,
		ModelProfiles:      cfg.ModelProfiles,
	}
}

// Hash returns a stable content hash of the snapshot.
func (s GPTAutoPolicySnapshot) Hash() (string, []byte, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), raw, nil
}

// ApplyTo overlays the snapshot onto a config so a replay evaluates against
// exactly the historical policy.
func (s GPTAutoPolicySnapshot) ApplyTo(cfg GPTAutoShadowConfig) GPTAutoShadowConfig {
	cfg.PolicyVersion = s.PolicyVersion
	cfg.GlobalMinMargin = s.GlobalMinMargin
	cfg.ProfitSafetyBuffer = s.ProfitSafetyBuffer
	cfg.ModelProfiles = s.ModelProfiles
	return cfg
}

// GPTAutoDecisionFilter filters the decision log.
type GPTAutoDecisionFilter struct {
	Pagination       pagination.PaginationParams
	StartTime        *time.Time
	EndTime          *time.Time
	ActualModel      string
	FinalModel       string
	FinalEffort      string
	EvaluationStatus string
	DecisionStatus   string
	GroupID          *int64
	RequestID        string
}

// GPTAutoStatsFilter scopes dashboard statistics.
type GPTAutoStatsFilter struct {
	StartTime time.Time
	EndTime   time.Time
	GroupID   *int64
}

// GPTAutoCount is one bucket of a distribution.
type GPTAutoCount struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// GPTAutoPairCount is one cell of the actual-vs-recommended matrix.
type GPTAutoPairCount struct {
	Actual      string `json:"actual"`
	Recommended string `json:"recommended"`
	Count       int64  `json:"count"`
}

// GPTAutoTrendPoint is one time bucket.
type GPTAutoTrendPoint struct {
	Bucket  time.Time `json:"bucket"`
	Success int64     `json:"success"`
	Failed  int64     `json:"failed"`
}

// GPTAutoStoredStats is what the repository aggregates from persisted rows.
type GPTAutoStoredStats struct {
	Total            int64    `json:"total"`
	Success          int64    `json:"success"`
	Failed           int64    `json:"failed"`
	AvgLatencyMS     *float64 `json:"avg_latency_ms,omitempty"`
	P50LatencyMS     *float64 `json:"p50_latency_ms,omitempty"`
	P95LatencyMS     *float64 `json:"p95_latency_ms,omitempty"`
	InputTokens      int64    `json:"input_tokens"`
	OutputTokens     int64    `json:"output_tokens"`
	ClassifierCost   float64  `json:"classifier_cost"`
	Adjusted         int64    `json:"adjusted"`
	NoEligible       int64    `json:"no_eligible"`
	BelowTargetCount int64    `json:"below_target_count"`
	// ModelBelowTargetCount counts model-only margins below target (including
	// losses); BelowTargetCount is fully loaded.
	ModelBelowTargetCount int64 `json:"model_below_target_count"`
	InsufficientEstimates int64 `json:"insufficient_estimates"`
	// PolicyModelChanges/PolicyEffortChanges count decisions where the policy
	// recommendation differs from Jev's original answer.
	PolicyModelChanges  int64 `json:"policy_model_changes"`
	PolicyEffortChanges int64 `json:"policy_effort_changes"`

	FinalModels  []GPTAutoCount `json:"final_models"`
	JevModels    []GPTAutoCount `json:"jev_models"`
	FinalEfforts []GPTAutoCount `json:"final_efforts"`
	JevEfforts   []GPTAutoCount `json:"jev_efforts"`
	// ClassifierConfigs counts successful decisions per classifier config hash
	// ("" = before versioning), so distributions can be compared across
	// instruction changes.
	ClassifierConfigs []GPTAutoCount      `json:"classifier_configs"`
	Complexity        []GPTAutoCount      `json:"complexity"`
	Categories        []GPTAutoCount      `json:"categories"`
	ErrorClasses      []GPTAutoCount      `json:"error_classes"`
	ActualVsFinal     []GPTAutoPairCount  `json:"actual_vs_final"`
	Trend             []GPTAutoTrendPoint `json:"trend"`
	TrendBucketSize   string              `json:"trend_bucket_size"`
}

// GPTAutoUsageToday backs the daily budget checks.
type GPTAutoUsageToday struct {
	Calls int64
	Cost  float64
}

// GPTAutoOutputHistory is one historical output-token aggregate from
// usage_logs.
type GPTAutoOutputHistory struct {
	Model     string
	Effort    string
	AvgOutput float64
	Samples   int64
}

// GPTAutoActualUsage is the real production outcome of the observed request,
// read from usage_logs at display time. Unlike the hypothetical economics it
// reflects what was actually served and charged.
type GPTAutoActualUsage struct {
	Model           string `json:"model"`
	RequestedModel  string `json:"requested_model"`
	UpstreamModel   string `json:"upstream_model"`
	ReasoningEffort string `json:"reasoning_effort"`
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	CacheReadTokens int64  `json:"cache_read_tokens"`
	// ListCost is total_cost: the request priced at list rates.
	ListCost float64 `json:"list_cost"`
	// ChargedCost is actual_cost: what the customer was billed.
	ChargedCost float64 `json:"charged_cost"`
	// AccountCost is the recorded upstream account cost basis
	// (account_stats_cost, else total_cost × account_rate_multiplier).
	AccountCost float64   `json:"account_cost"`
	CreatedAt   time.Time `json:"created_at"`
}

// GPTAutoShadowRepository persists shadow decisions.
type GPTAutoShadowRepository interface {
	Insert(ctx context.Context, decision *GPTAutoShadowDecision) error
	UpsertPolicySnapshot(ctx context.Context, hash, policyVersion string, snapshot []byte) error
	GetPolicySnapshot(ctx context.Context, hash string) (*GPTAutoPolicySnapshot, error)
	// UpsertClassifierConfig stores a classifier configuration version; an
	// existing hash keeps its original revision, author and timestamp.
	UpsertClassifierConfig(ctx context.Context, record GPTAutoClassifierConfigRecord) error
	GetClassifierConfig(ctx context.Context, hash string) (*GPTAutoClassifierConfigRecord, error)
	ListClassifierConfigs(ctx context.Context, limit int) ([]GPTAutoClassifierConfigRecord, error)
	GetByID(ctx context.Context, id int64) (*GPTAutoShadowDecision, error)
	List(ctx context.Context, filter GPTAutoDecisionFilter) ([]GPTAutoShadowDecision, *pagination.PaginationResult, error)
	Stats(ctx context.Context, filter GPTAutoStatsFilter) (*GPTAutoStoredStats, error)
	UsageSince(ctx context.Context, since time.Time) (GPTAutoUsageToday, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time, batchSize int) (int64, error)
	// OutputHistory aggregates real output-token usage from usage_logs for the
	// given models since a time. Used only for labelled cost estimates.
	OutputHistory(ctx context.Context, models []string, since time.Time) ([]GPTAutoOutputHistory, error)
	// ActualUsage finds the usage_logs row for a request near a time. It
	// returns nil, nil when none exists (failed request, not yet recorded).
	ActualUsage(ctx context.Context, requestID string, around time.Time) (*GPTAutoActualUsage, error)
}
