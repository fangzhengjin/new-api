package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetQuotaDataGroupByUserResolvesDisplayNames(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", DisplayName: "Alice D.", AffCode: "aff-alice"}).Error)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "bob", AffCode: "aff-bob"}).Error)
	require.NoError(t, DB.Create(&QuotaData{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 1000,
		Count:     2,
		Quota:     100,
	}).Error)
	require.NoError(t, DB.Create(&QuotaData{
		UserID:    2,
		Username:  "bob",
		ModelName: "gpt-b",
		CreatedAt: 1100,
		Count:     1,
		Quota:     50,
	}).Error)

	rows, err := GetQuotaDataGroupByUser(900, 2000)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// Rows stay keyed by username; only the display label is enriched, and an
	// empty display name leaves the field unset for the username fallback.
	byUsername := make(map[string]*QuotaData, len(rows))
	for _, row := range rows {
		byUsername[row.Username] = row
	}
	require.Equal(t, "Alice D.", byUsername["alice"].DisplayName)
	require.Empty(t, byUsername["bob"].DisplayName)
}
