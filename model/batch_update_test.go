package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBatchUpdateRequeuesFailedDeltas(t *testing.T) {
	originalDB := DB
	originalStores := batchUpdateStores
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	batchUpdateStores = make([]map[int]int, BatchUpdateTypeCount)
	for i := range batchUpdateStores {
		batchUpdateStores[i] = make(map[int]int)
	}
	t.Cleanup(func() {
		DB = originalDB
		batchUpdateStores = originalStores
	})

	addNewRecord(BatchUpdateTypeTokenQuota, 11, 3)
	addNewRecord(BatchUpdateTypeChannelUsedQuota, 22, 4)
	addNewRecord(BatchUpdateTypeUserQuota, 33, -5)
	addNewRecord(BatchUpdateTypeUsedQuota, 33, 6)
	addNewRecord(BatchUpdateTypeRequestCount, 33, 7)

	batchUpdate()

	require.Equal(t, 3, batchUpdateStores[BatchUpdateTypeTokenQuota][11])
	require.Equal(t, 4, batchUpdateStores[BatchUpdateTypeChannelUsedQuota][22])
	require.Equal(t, -5, batchUpdateStores[BatchUpdateTypeUserQuota][33])
	require.Equal(t, 6, batchUpdateStores[BatchUpdateTypeUsedQuota][33])
	require.Equal(t, 7, batchUpdateStores[BatchUpdateTypeRequestCount][33])
}
