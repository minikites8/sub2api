package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestRedeemCodeFromServiceRedactsRiskControlNotes(t *testing.T) {
	record := &service.RedeemCode{
		Type:  service.AdjustmentTypeRiskControlBalance,
		Notes: "API IP+UA 风控扣除赠金（IP: 192.0.2.1）",
	}

	userDTO := RedeemCodeFromService(record)
	require.NotNil(t, userDTO)
	require.Nil(t, userDTO.Notes)

	adminDTO := RedeemCodeFromServiceAdmin(record)
	require.NotNil(t, adminDTO)
	require.Empty(t, adminDTO.Notes)
}

func TestRedeemCodeFromServiceKeepsAdminAdjustmentNotes(t *testing.T) {
	record := &service.RedeemCode{
		Type:  "admin_balance",
		Notes: "service credit",
	}

	userDTO := RedeemCodeFromService(record)
	require.NotNil(t, userDTO)
	require.NotNil(t, userDTO.Notes)
	require.Equal(t, "service credit", *userDTO.Notes)

	adminDTO := RedeemCodeFromServiceAdmin(record)
	require.NotNil(t, adminDTO)
	require.Equal(t, "service credit", adminDTO.Notes)
}
