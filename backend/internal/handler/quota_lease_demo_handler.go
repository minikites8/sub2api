package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type QuotaLeaseDemoHandler struct {
	svc *service.QuotaLeaseDemoService
}

func NewQuotaLeaseDemoHandler(svc *service.QuotaLeaseDemoService) *QuotaLeaseDemoHandler {
	return &QuotaLeaseDemoHandler{svc: svc}
}

func (h *QuotaLeaseDemoHandler) RegisterNode(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var req service.QuotaLeaseDemoNodeRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	result, err := h.svc.RegisterNode(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *QuotaLeaseDemoHandler) HeartbeatNode(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoNodeHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if req.NodeID == "" {
		req.NodeID = nodeID
	}
	node, err := h.svc.HeartbeatNode(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"node": node})
}

func (h *QuotaLeaseDemoHandler) ListNodes(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": h.svc.ListNodes()})
}

func (h *QuotaLeaseDemoHandler) CreateAccountLoginTask(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var req service.QuotaLeaseDemoAccountLoginTaskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	task, err := h.svc.CreateAccountLoginTask(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *QuotaLeaseDemoHandler) ListAccountLoginTasks(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	requestedNodeID := strings.TrimSpace(c.Query("node_id"))
	nodeID, ok := h.authenticateNodeOrControl(c, requestedNodeID)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"tasks": h.svc.ListAccountLoginTasks(c.Request.Context(), nodeID, c.Query("status")),
	})
}

func (h *QuotaLeaseDemoHandler) CompleteAccountLoginTask(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoAccountLoginTaskCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if req.TaskID == "" {
		req.TaskID = c.Param("task_id")
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if req.NodeID == "" {
		req.NodeID = nodeID
	}
	task, err := h.svc.CompleteAccountLoginTask(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *QuotaLeaseDemoHandler) ReportAccountLoginTaskProgress(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoAccountLoginTaskProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if req.TaskID == "" {
		req.TaskID = c.Param("task_id")
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if req.NodeID == "" {
		req.NodeID = nodeID
	}
	task, err := h.svc.ReportAccountLoginTaskProgress(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *QuotaLeaseDemoHandler) SubmitAccountLoginTaskCallback(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	var req service.QuotaLeaseDemoAccountLoginTaskCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	if req.TaskID == "" {
		req.TaskID = c.Param("task_id")
	}
	task, err := h.svc.SubmitAccountLoginTaskCallback(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *QuotaLeaseDemoHandler) ReportAccountStatus(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoAccountStatusReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if req.NodeID == "" {
		req.NodeID = nodeID
	}
	account, err := h.svc.ReportAccountStatus(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"account": account})
}

func (h *QuotaLeaseDemoHandler) ListAssignedAccounts(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	requestedNodeID := strings.TrimSpace(c.Query("node_id"))
	nodeID, ok := h.authenticateNodeOrControl(c, requestedNodeID)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"accounts": h.svc.ListAssignedAccounts(c.Request.Context(), nodeID),
	})
}

func (h *QuotaLeaseDemoHandler) RequestLease(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoLeaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if req.NodeID == "" {
		req.NodeID = nodeID
	}
	lease, err := h.svc.RequestLease(c.Request.Context(), req)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"lease": lease})
}

func (h *QuotaLeaseDemoHandler) PostUsageBatch(c *gin.Context) {
	if !h.requireEnabled(c) {
		return
	}
	var req service.QuotaLeaseDemoUsageBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}
	nodeID, ok := h.authenticateNodeOrControl(c, req.NodeID)
	if !ok {
		return
	}
	if nodeID != "" {
		if req.NodeID == "" {
			req.NodeID = nodeID
		}
		if req.NodeID != nodeID {
			c.JSON(http.StatusForbidden, gin.H{"error": "node_mismatch"})
			return
		}
		for _, event := range req.Events {
			if strings.TrimSpace(event.NodeID) != "" && strings.TrimSpace(event.NodeID) != nodeID {
				c.JSON(http.StatusForbidden, gin.H{"error": "node_mismatch"})
				return
			}
		}
	}
	c.JSON(http.StatusOK, h.svc.PostUsageBatch(c.Request.Context(), req))
}

func (h *QuotaLeaseDemoHandler) ReclaimExpired(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	c.JSON(http.StatusOK, h.svc.ReclaimExpired(c.Request.Context(), time.Now().UTC()))
}

func (h *QuotaLeaseDemoHandler) Status(c *gin.Context) {
	if !h.requireEnabled(c) || !h.requireControlSecret(c) {
		return
	}
	c.JSON(http.StatusOK, h.svc.Snapshot())
}

func (h *QuotaLeaseDemoHandler) requireEnabled(c *gin.Context) bool {
	if h == nil || h.svc == nil || !h.svc.Enabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "quota_lease_demo_disabled"})
		return false
	}
	return true
}

func (h *QuotaLeaseDemoHandler) requireControlSecret(c *gin.Context) bool {
	if h.controlSecretOK(c) {
		return true
	}
	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_node_secret"})
	return false
}

func (h *QuotaLeaseDemoHandler) controlSecretOK(c *gin.Context) bool {
	secret := h.svc.NodeSecret()
	if secret == "" {
		return true
	}
	return strings.TrimSpace(c.GetHeader("X-Node-Secret")) == secret
}

func (h *QuotaLeaseDemoHandler) authenticateNodeOrControl(c *gin.Context, requestedNodeID string) (string, bool) {
	requestedNodeID = strings.TrimSpace(requestedNodeID)
	headerNodeID := strings.TrimSpace(c.GetHeader("X-Node-ID"))
	nodeID := headerNodeID
	if nodeID == "" {
		nodeID = requestedNodeID
	}
	nodeSecret := strings.TrimSpace(c.GetHeader("X-Node-Secret"))
	if nodeID != "" && h.svc.AuthenticateNode(nodeID, nodeSecret) {
		if requestedNodeID != "" && requestedNodeID != nodeID {
			c.JSON(http.StatusForbidden, gin.H{"error": "node_mismatch"})
			return "", false
		}
		return nodeID, true
	}
	if h.controlSecretOK(c) {
		return requestedNodeID, true
	}
	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_node_secret"})
	return "", false
}

func (h *QuotaLeaseDemoHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrQuotaLeaseDemoDisabled):
		c.JSON(http.StatusNotFound, gin.H{"error": "quota_lease_demo_disabled"})
	case errors.Is(err, service.ErrQuotaLeaseDemoInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
	case errors.Is(err, service.ErrQuotaLeaseDemoConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "event_conflict", "message": err.Error()})
	case errors.Is(err, service.ErrQuotaLeaseDemoNodeNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "node_not_found", "message": err.Error()})
	case errors.Is(err, service.ErrQuotaLeaseDemoNoCapacity):
		c.JSON(http.StatusForbidden, gin.H{"error": "no_capacity", "message": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "message": err.Error()})
	}
}
