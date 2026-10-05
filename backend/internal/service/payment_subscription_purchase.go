package service

import (
	"math"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
)

const MaxSubscriptionPurchaseQuantity = 1000

type subscriptionPurchase struct {
	Quantity int
	Days     int
	Amount   float64
}

func quoteSubscriptionPurchase(plan *dbent.SubscriptionPlan, quantity int) (subscriptionPurchase, error) {
	if quantity == 0 {
		quantity = 1 // Requests from older clients purchase one period.
	}
	if quantity < 1 || quantity > MaxSubscriptionPurchaseQuantity {
		return subscriptionPurchase{}, infraerrors.BadRequest("INVALID_QUANTITY", "subscription quantity must be between 1 and 1000")
	}
	factor := psComputeValidityDays(1, plan.ValidityUnit)
	if plan.ValidityDays <= 0 || plan.ValidityDays > MaxValidityDays/factor/quantity {
		return subscriptionPurchase{}, infraerrors.BadRequest("INVALID_QUANTITY", "total subscription duration exceeds the allowed range")
	}
	if math.IsNaN(plan.Price) || math.IsInf(plan.Price, 0) || plan.Price <= 0 {
		return subscriptionPurchase{}, infraerrors.BadRequest("INVALID_AMOUNT", "subscription price must be a positive number")
	}
	amount, _ := decimal.NewFromFloat(plan.Price).Mul(decimal.NewFromInt(int64(quantity))).Round(8).Float64()
	if math.IsInf(amount, 0) || amount <= 0 {
		return subscriptionPurchase{}, infraerrors.BadRequest("INVALID_AMOUNT", "subscription total must be a positive finite number")
	}
	return subscriptionPurchase{Quantity: quantity, Days: plan.ValidityDays * factor * quantity, Amount: amount}, nil
}

// PaymentOrderQuantity reads the immutable purchase snapshot; legacy orders contain one period.
func PaymentOrderQuantity(order *dbent.PaymentOrder) int {
	if order == nil || order.OrderType != payment.OrderTypeSubscription {
		return 0
	}
	if quantity, ok := snapshotInt64(order.ProviderSnapshot["subscription_quantity"]); ok && quantity >= 1 && quantity <= MaxSubscriptionPurchaseQuantity {
		return int(quantity)
	}
	return 1
}
