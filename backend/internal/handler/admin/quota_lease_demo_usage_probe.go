package admin

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func runQuotaLeaseDemoUsageProbeTaskForAdmin(
	ctx context.Context,
	adminSvc service.AdminService,
	quotaSvc *service.QuotaLeaseDemoService,
	accountID int64,
	source string,
	force bool,
	probeKind string,
) (*service.QuotaLeaseDemoUsageProbeTask, bool, error) {
	if adminSvc == nil || quotaSvc == nil || !quotaSvc.Enabled() {
		return nil, false, nil
	}
	if strings.TrimSpace(quotaSvc.ControlPlaneBaseURL()) != "" || strings.TrimSpace(quotaSvc.RegistrationURL()) != "" {
		return nil, false, nil
	}
	account, err := adminSvc.GetAccount(ctx, accountID)
	if err != nil {
		return nil, true, err
	}
	if account == nil {
		return nil, true, service.ErrAccountNotFound
	}
	nodeID := service.QuotaLeaseDemoAssignedNodeID(*account)
	if nodeID == "" {
		return nil, false, nil
	}
	task, err := quotaSvc.CreateUsageProbeTask(ctx, service.QuotaLeaseDemoUsageProbeTaskCreateRequest{
		AccountID:      accountID,
		AssignedNodeID: nodeID,
		Platform:       account.Platform,
		Source:         source,
		Force:          force,
		ProbeKind:      probeKind,
	})
	if err != nil {
		return nil, true, err
	}
	task, err = quotaSvc.WaitUsageProbeTask(ctx, task.ID, 0, 0)
	if err != nil {
		return task, true, err
	}
	return task, true, nil
}
