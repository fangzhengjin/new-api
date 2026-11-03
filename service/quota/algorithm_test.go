package quota

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustQuota(t *testing.T, value string) int64 {
	t.Helper()
	amount, ok := new(big.Rat).SetString(value)
	require.True(t, ok)
	amount.Mul(amount, new(big.Rat).SetInt64(defaultQuotaPerUnit))
	require.True(t, amount.IsInt())
	require.True(t, amount.Num().IsInt64())
	return amount.Num().Int64()
}

func TestPositiveQuotaParsingRejectsInvalidAndOverflowingValues(t *testing.T) {
	t.Parallel()

	quota, err := ParsePositiveQuota("50005000", "额度")
	require.NoError(t, err)
	assert.Equal(t, int64(50_005_000), quota)
	assert.Equal(t, "＄100.010000", FormatQuota(quota))
	for _, value := range []string{"", ".5", "1.", "1.001", "0", "-1", "abc"} {
		_, err := ParsePositiveQuota(value, "额度")
		assert.Error(t, err, value)
	}
	_, err = ParsePositiveQuota("999999999999999999999999", "额度")
	assert.Error(t, err)
	_, err = bigRatio([]int64{math.MaxInt64, math.MaxInt64}, nil, false)
	assert.ErrorIs(t, err, errQuotaOverflow)
}

func TestQuotaUnitFollowsSystemSetting(t *testing.T) {
	original := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = original })
	common.QuotaPerUnit = 200_000

	assert.Equal(t, int64(200_000), quotaPerUnit())
	rounded, err := roundUpCent(1)
	require.NoError(t, err)
	assert.Equal(t, int64(2_000), rounded)
}

func TestMaterialAdjustmentThreshold(t *testing.T) {
	t.Parallel()

	assert.False(t, isMaterialAdjustment(mustQuota(t, "1"), mustQuota(t, "1")))
	assert.True(t, isMaterialAdjustment(mustQuota(t, "1.01"), mustQuota(t, "1")))
}

func TestFairAllocationMaximizesLowestDemandCoverage(t *testing.T) {
	t.Parallel()
	allocations, err := allocateFairRequests([]requestRow{
		{UserID: 1, Balance: mustQuota(t, "10"), Target: mustQuota(t, "100"), Requested: mustQuota(t, "90")},
		{UserID: 2, Balance: mustQuota(t, "90"), Target: mustQuota(t, "100"), Requested: mustQuota(t, "10")},
	}, mustQuota(t, "40"), mustQuota(t, "1"))
	require.NoError(t, err)
	assert.Equal(t, mustQuota(t, "40"), allocations[1])
	assert.Zero(t, allocations[2])
}

func TestFairAllocationCompletesSafetyLayerBeforeDemandLayer(t *testing.T) {
	t.Parallel()
	allocations, err := allocateFairRequests([]requestRow{
		{UserID: 1, SafetyTarget: mustQuota(t, "50"), Target: mustQuota(t, "100"), Requested: mustQuota(t, "100")},
		{UserID: 2, Target: mustQuota(t, "100"), Requested: mustQuota(t, "100")},
	}, mustQuota(t, "60"), mustQuota(t, "1"))
	require.NoError(t, err)
	assert.Equal(t, mustQuota(t, "50"), allocations[1])
	assert.Equal(t, mustQuota(t, "10"), allocations[2])
}

func TestFairAllocationUsesWaitTimeWhenMinimumUnitsAreScarce(t *testing.T) {
	t.Parallel()
	allocations, err := allocateFairRequests([]requestRow{
		{UserID: 1, Target: mustQuota(t, "100"), Requested: mustQuota(t, "100"), LastPositiveAt: 200},
		{UserID: 2, Target: mustQuota(t, "100"), Requested: mustQuota(t, "100"), LastPositiveAt: 100},
	}, mustQuota(t, "1.01"), mustQuota(t, "1"))
	require.NoError(t, err)
	assert.NotContains(t, allocations, 1)
	assert.Equal(t, mustQuota(t, "1.01"), allocations[2])
}

func TestFairAllocationPrioritizesUnmetSafetyWhenMinimumUnitsAreScarce(t *testing.T) {
	t.Parallel()
	allocations, err := allocateFairRequests([]requestRow{
		{UserID: 1, Target: mustQuota(t, "100"), Requested: mustQuota(t, "100")},
		{UserID: 2, Balance: mustQuota(t, "90"), SafetyTarget: mustQuota(t, "100"), Target: mustQuota(t, "100"), Requested: mustQuota(t, "10")},
	}, mustQuota(t, "1.01"), mustQuota(t, "1"))
	require.NoError(t, err)
	assert.NotContains(t, allocations, 1)
	assert.Equal(t, mustQuota(t, "1.01"), allocations[2])
}

func TestFairAllocationHandlesEmptySingleAndZeroCapacity(t *testing.T) {
	t.Parallel()
	empty, err := allocateFairRequests(nil, mustQuota(t, "10"), mustQuota(t, "1"))
	require.NoError(t, err)
	assert.Empty(t, empty)

	zero, err := allocateFairRequests([]requestRow{{UserID: 1, Requested: mustQuota(t, "100")}}, 0, mustQuota(t, "1"))
	require.NoError(t, err)
	assert.Empty(t, zero)

	single, err := allocateFairRequests([]requestRow{
		{UserID: 1, Target: mustQuota(t, "100"), Requested: mustQuota(t, "100")},
	}, mustQuota(t, "40"), mustQuota(t, "1"))
	require.NoError(t, err)
	assert.Equal(t, mustQuota(t, "40"), single[1])
}

func TestDemandAndReclaimRuleBoundaries(t *testing.T) {
	t.Parallel()

	assert.Equal(t, mustQuota(t, "160"), mustRecommendInitialGrant(t, mustQuota(t, "16900"), 169, mustQuota(t, "200")))
	assert.Equal(t, mustQuota(t, "180"), mustRecommendInitialGrant(t, mustQuota(t, "25350"), 169, mustQuota(t, "200")))
	assert.Equal(t, int64(0), availableIncreaseCap(mustQuota(t, "375"), mustQuota(t, "400"), mustQuota(t, "20")))
	assert.Equal(t, mustQuota(t, "5"), availableIncreaseCap(mustQuota(t, "375"), mustQuota(t, "400"), mustQuota(t, "30")))
	assert.Equal(t, mustQuota(t, "375"), availableIncreaseCap(mustQuota(t, "375"), 0, mustQuota(t, "20")))
	assert.False(t, isOrdinaryLowUsage(mustQuota(t, "56.09"), mustQuota(t, "56.09"), mustQuota(t, "200")))
	assert.True(t, isOrdinaryLowUsage(0, mustQuota(t, "80"), mustQuota(t, "200")))
	assert.True(t, isOrdinaryLowUsage(mustQuota(t, "9"), mustQuota(t, "20"), mustQuota(t, "200")))
}

func TestGenerateLogContentIncludesAdjustmentEvidence(t *testing.T) {
	t.Parallel()
	cycle := model.QuotaCycle{
		CycleStartAt: mustTimestamp(t, "2026-07-07T00:00:00+08:00"),
		CycleEndAt:   mustTimestamp(t, "2026-08-07T00:00:00+08:00"),
	}
	content := GenerateLogContent(model.QuotaItem{
		Action:          model.QuotaAdjustmentActionDecrease,
		AdjustmentQuota: -mustQuota(t, "139.13"),
		BasisText:       "本期无使用记录\n自上一份已执行方案以来已完成观察",
	}, mustQuota(t, "200"), mustQuota(t, "60.87"), cycle)

	assert.Contains(t, content, "调整依据：\n1. 本期无使用记录\n2. 自上一份已执行方案以来已完成观察")
	assert.Contains(t, content, "本次调减：＄139.130000")
	assert.Contains(t, content, "注：如后续用量增加，将在下次额度调配时重新核算。")
}

func mustRecommendInitialGrant(t *testing.T, totalSpend int64, userCount int, previousInitial int64) int64 {
	t.Helper()
	quota, err := recommendInitialGrant(totalSpend, userCount, previousInitial)
	require.NoError(t, err)
	return quota
}

func mustTimestamp(t *testing.T, value string) int64 {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed.Unix()
}
