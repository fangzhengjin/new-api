package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecompressRequestMiddlewareZstd(t *testing.T) {
	payload := []byte(`{"model":"gpt-5","input":"hello"}`)
	encoder, err := zstd.NewWriter(nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = encoder.Close() })

	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(encoder.EncodeAll(payload, nil)))
	request.Header.Set("Content-Encoding", "zstd")
	recorder := httptest.NewRecorder()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(DecompressRequestMiddleware())
	router.POST("/", func(c *gin.Context) {
		body, readErr := io.ReadAll(c.Request.Body)
		require.NoError(t, readErr)
		assert.Equal(t, payload, body)
		assert.Empty(t, c.GetHeader("Content-Encoding"))
		c.Status(http.StatusNoContent)
	})

	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}
