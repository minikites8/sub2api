package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const autoSupplyOrderHistoryLimit = 200

// AutoSupplyOrderRecord is the administrator-facing history of upstream
// replenishment orders. Credentials and bundle contents are never stored.
type AutoSupplyOrderRecord struct {
	ID        string    `json:"id"`
	GroupID   int64     `json:"group_id"`
	Product   string    `json:"product"`
	Quantity  int       `json:"quantity"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *AutoSupplyService) ListOrders(ctx context.Context) ([]AutoSupplyOrderRecord, error) {
	if s == nil || s.settingRepo == nil {
		return []AutoSupplyOrderRecord{}, nil
	}
	s.orderHistoryMu.Lock()
	defer s.orderHistoryMu.Unlock()
	return s.loadOrderHistory(ctx)
}

func (s *AutoSupplyService) recordOrder(ctx context.Context, order *autoSupplyOrder, status, orderError string) error {
	if s == nil || s.settingRepo == nil || order == nil || strings.TrimSpace(order.ID) == "" {
		return nil
	}
	now := time.Now().UTC()
	if order.CreatedAt.IsZero() {
		order.CreatedAt = now
	}
	order.Status = strings.TrimSpace(status)
	if order.Status == "" {
		order.Status = "pending"
	}
	order.LastError = strings.TrimSpace(orderError)
	order.UpdatedAt = now

	record := AutoSupplyOrderRecord{
		ID: order.ID, GroupID: order.GroupID, Product: order.Product, Quantity: order.Quantity,
		Status: order.Status, Error: order.LastError, CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt,
	}
	s.orderHistoryMu.Lock()
	defer s.orderHistoryMu.Unlock()
	records, err := s.loadOrderHistory(ctx)
	if err != nil {
		return err
	}
	replaced := false
	for index := range records {
		if records[index].ID == record.ID {
			records[index] = record
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, record)
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].UpdatedAt.After(records[j].UpdatedAt) })
	if len(records) > autoSupplyOrderHistoryLimit {
		records = records[:autoSupplyOrderHistoryLimit]
	}
	raw, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("marshal auto supply order history: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyAutoSupplyOrders, string(raw)); err != nil {
		return fmt.Errorf("save auto supply order history: %w", err)
	}
	return nil
}

func (s *AutoSupplyService) loadOrderHistory(ctx context.Context) ([]AutoSupplyOrderRecord, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAutoSupplyOrders)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return []AutoSupplyOrderRecord{}, nil
		}
		return nil, fmt.Errorf("load auto supply order history: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return []AutoSupplyOrderRecord{}, nil
	}
	var records []AutoSupplyOrderRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, fmt.Errorf("decode auto supply order history: %w", err)
	}
	if records == nil {
		records = []AutoSupplyOrderRecord{}
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].UpdatedAt.After(records[j].UpdatedAt) })
	if len(records) > autoSupplyOrderHistoryLimit {
		records = records[:autoSupplyOrderHistoryLimit]
	}
	return records, nil
}
