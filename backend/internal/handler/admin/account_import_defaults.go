package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const adminAPIKeySessionCleanupTimeout = 2 * time.Minute

func scheduleAdminAPIKeySessionCleanup(ctx context.Context, sessions openAISessionService, account *service.Account) {
	defaults, ok := service.AdminAPIKeyAccountDefaultsFromContext(ctx)
	if !ok || !defaults.RevokeOtherSessions || sessions == nil || account == nil ||
		account.Platform != service.PlatformOpenAI || account.Type != service.AccountTypeOAuth {
		return
	}
	accountID := account.ID
	go func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), adminAPIKeySessionCleanupTimeout)
		defer cancel()
		revoked, err := revokeOtherOpenAISessions(cleanupCtx, sessions, accountID)
		if err != nil {
			slog.Warn("admin_api_key_session_cleanup_failed", "account_id", accountID, "revoked", revoked, "error", err)
			return
		}
		slog.Info("admin_api_key_session_cleanup_completed", "account_id", accountID, "revoked", revoked)
	}()
}

func revokeOtherOpenAISessions(ctx context.Context, sessions openAISessionService, accountID int64) (int, error) {
	listed, err := sessions.ListSessions(ctx, accountID)
	if err != nil {
		return 0, err
	}
	revoked := 0
	var revokeErrors []error
	for _, device := range listed.Devices {
		sessionID := strings.TrimSpace(device.SessionID)
		if sessionID == "" || device.IsCurrentDevice {
			continue
		}
		if _, err := sessions.RevokeSession(ctx, accountID, sessionID); err != nil {
			revokeErrors = append(revokeErrors, fmt.Errorf("revoke session %s: %w", sessionID, err))
			continue
		}
		revoked++
	}
	return revoked, errors.Join(revokeErrors...)
}
