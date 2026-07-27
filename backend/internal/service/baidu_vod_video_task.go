package service

import (
	"context"
	"time"
)

const (
	BaiduVODTaskStatusSubmitting = "submitting"
	BaiduVODTaskStatusQueued     = "queued"
	BaiduVODTaskStatusInProgress = "in_progress"
	BaiduVODTaskStatusSettling   = "settling"
	BaiduVODTaskStatusCompleted  = "completed"
	BaiduVODTaskStatusFailed     = "failed"
)

type BaiduVODVideoTask struct {
	ID                    int64
	Platform              string
	Provider              string
	TaskID                string
	UpstreamTaskID        string
	UpstreamRequestID     *string
	UserID                int64
	APIKeyID              int64
	AccountID             int64
	GroupID               *int64
	Model                 string
	UpstreamModel         string
	Capability            BaiduVODVideoCapability
	Status                string
	UpstreamStatus        string
	Resolution            string
	Ratio                 string
	RequestedDuration     int
	OutputDuration        int
	InputVideoDuration    int
	InputContainsVideo    bool
	VideoCount            int
	BillingMode           string
	EstimatedCost         float64
	HoldAmount            float64
	ActualCost            *float64
	GroupRateMultiplier   float64
	VideoRateMultiplier   float64
	AccountRateMultiplier float64
	RequestHash           string
	ResultURL             *string
	ResultExpiresAt       *time.Time
	LastErrorCode         *string
	LastErrorMessage      *string
	RetryCount            int
	Version               int
	NextPollAt            time.Time
	PollClaimedUntil      *time.Time
	LastPolledAt          *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
	SubmittedAt           time.Time
	StartedAt             *time.Time
	FinishedAt            *time.Time
	SettledAt             *time.Time
}

type BaiduVODVideoTaskPollUpdate struct {
	Version            int
	Status             string
	UpstreamStatus     string
	ResultURL          *string
	ResultExpiresAt    *time.Time
	OutputDuration     int
	InputVideoDuration int
	VideoCount         int
	ActualCost         *float64
	LastErrorCode      *string
	LastErrorMessage   *string
	NextPollAt         time.Time
	RetryCount         int
	FinishedAt         *time.Time
	SettledAt          *time.Time
}

type BaiduVODVideoTaskRepository interface {
	Create(ctx context.Context, task *BaiduVODVideoTask) error
	GetByTaskID(ctx context.Context, taskID string) (*BaiduVODVideoTask, error)
	GetForOwner(ctx context.Context, userID, apiKeyID int64, taskID string) (*BaiduVODVideoTask, error)
	ListDue(ctx context.Context, now time.Time, limit int) ([]*BaiduVODVideoTask, error)
	Claim(ctx context.Context, taskID string, version int, until time.Time) (bool, error)
	UpdatePoll(ctx context.Context, taskID string, update BaiduVODVideoTaskPollUpdate) (bool, error)
	MarkSubmitted(ctx context.Context, taskID string, submitted BaiduVODSubmitResult, nextPollAt time.Time) (bool, error)
	MarkSubmissionFailed(ctx context.Context, taskID, code, message string, finishedAt time.Time) (bool, error)
}

func BaiduVODTaskIsTerminal(status string) bool {
	return status == BaiduVODTaskStatusCompleted || status == BaiduVODTaskStatusFailed
}
