package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionsBulkRejectsInvalidJSONBeforeWriting(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "SMTPServer", Value: "old.example.com"}).Error)
	DB = db
	t.Cleanup(func() { DB = previousDB })

	err = UpdateOptionsBulk(map[string]string{
		"SMTPServer": "new.example.com",
		"ModelPrice": "{invalid",
	})
	require.Error(t, err)

	var option Option
	require.NoError(t, db.First(&option, "key = ?", "SMTPServer").Error)
	assert.Equal(t, "old.example.com", option.Value)
	assert.ErrorIs(t, db.First(&Option{}, "key = ?", "ModelPrice").Error, gorm.ErrRecordNotFound)
}
