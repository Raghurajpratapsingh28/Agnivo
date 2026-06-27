package billing_test

import (
	"context"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/billing"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore satisfies the subset of *store.Repository that billing tests need.
// We use a real DB in integration; here we verify purely the engine logic.

func TestNopProvider(t *testing.T) {
	ctx := context.Background()
	np := billing.NopProvider{}

	cid, err := np.CreateCustomer(ctx, "org1", "test@example.com", "Test Org")
	require.NoError(t, err)
	assert.Contains(t, cid, "cus_nop_")

	sid, err := np.CreateSubscription(ctx, cid, "price_pro_monthly", 14)
	require.NoError(t, err)
	assert.Contains(t, sid, "sub_nop_")

	require.NoError(t, np.UpdateSubscription(ctx, sid, "price_team_monthly", true))
	require.NoError(t, np.CancelSubscription(ctx, sid, false))

	invID, err := np.CreateInvoice(ctx, cid, 2000, "Test invoice")
	require.NoError(t, err)
	assert.Contains(t, invID, "inv_nop_")

	status, err := np.GetInvoiceStatus(ctx, invID)
	require.NoError(t, err)
	assert.Equal(t, "paid", status)
}

func TestPlanIDs(t *testing.T) {
	assert.Equal(t, model.PlanID("free"), model.PlanFree)
	assert.Equal(t, model.PlanID("pro"), model.PlanPro)
	assert.Equal(t, model.PlanID("team"), model.PlanTeam)
	assert.Equal(t, model.PlanID("enterprise"), model.PlanEnterprise)
}

func TestBillingIntervals(t *testing.T) {
	assert.Equal(t, model.BillingInterval("monthly"), model.IntervalMonthly)
	assert.Equal(t, model.BillingInterval("yearly"), model.IntervalYearly)
}
