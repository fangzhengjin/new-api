package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllLogsEnrichesCurrentDisplayName(t *testing.T) {
	truncateTables(t)

	user := &User{
		Id:          1,
		Username:    "current-alice",
		Password:    "password123",
		DisplayName: "Alice Chen",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "alice-aff",
	}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:    user.Id,
		Username:  "historical-alice",
		CreatedAt: 1000,
		Type:      LogTypeConsume,
	}).Error)

	logs, total, err := GetAllLogs(
		LogTypeUnknown,
		0,
		0,
		"",
		"",
		"",
		0,
		20,
		0,
		"",
		"",
		"",
	)

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, "historical-alice", logs[0].Username)
	assert.Equal(t, "Alice Chen", logs[0].DisplayName)
}
