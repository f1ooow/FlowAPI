package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetChannelPassthroughSettings(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := *settings
	t.Cleanup(func() { *settings = original })

	for _, tc := range []struct {
		name        string
		key         string
		value       bool
		wantBody    bool
		wantHeaders bool
	}{
		{name: "body only", key: "pass_through_request_enabled", value: true, wantBody: true},
		{name: "both enabled", key: "pass_through_headers_enabled", value: true, wantBody: true, wantHeaders: true},
		{name: "headers only", key: "pass_through_request_enabled", wantHeaders: true},
		{name: "both disabled", key: "pass_through_headers_enabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings.PassThroughRequestEnabled = tc.wantBody
			settings.PassThroughHeadersEnabled = tc.wantHeaders
			if tc.key == "pass_through_request_enabled" {
				settings.PassThroughRequestEnabled = !tc.value
			} else {
				settings.PassThroughHeadersEnabled = !tc.value
			}
			require.NoError(t, config.UpdateConfigFromMap(config.GlobalConfig.Get("global"), map[string]string{
				tc.key: strconv.FormatBool(tc.value),
			}))
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			GetChannelPassthroughSettings(ctx)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool           `json:"success"`
				Data    map[string]any `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.True(t, response.Success)
			assert.Equal(t, map[string]any{
				"pass_through_request_enabled": tc.wantBody,
				"pass_through_headers_enabled": tc.wantHeaders,
			}, response.Data)
		})
	}
}
