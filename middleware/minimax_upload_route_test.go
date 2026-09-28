package middleware

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMiniMaxUploadRouteChannels(t *testing.T) {
	t.Helper()
	require.NoError(t, i18n.Init())
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	previousDB, previousCache := model.DB, common.MemoryCacheEnabled
	model.DB, common.MemoryCacheEnabled = db, true
	t.Cleanup(func() {
		model.DB, common.MemoryCacheEnabled = previousDB, previousCache
		if previousCache && previousDB != nil && previousDB.Migrator().HasTable(&model.Channel{}) && previousDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	// Mirrors production: MetaSo H3 outranks the official account that owns cloning.
	for _, ch := range []struct {
		id       int
		priority int64
		models   string
	}{
		{80, 10, "MiniMax-H3,minimax-files-upload"},
		{71, 0, "minimax-voice-clone,minimax-files-upload,speech-02-hd"},
	} {
		priority, weight := ch.priority, uint(1)
		require.NoError(t, db.Create(&model.Channel{Id: ch.id, Type: constant.ChannelTypeMiniMax, Key: fmt.Sprintf("key-%d", ch.id), Status: common.ChannelStatusEnabled, Name: fmt.Sprintf("minimax-%d", ch.id), Weight: &weight, Models: ch.models, Group: "vedio", Priority: &priority}).Error)
		for _, modelName := range strings.Split(ch.models, ",") {
			require.NoError(t, db.Create(&model.Ability{Group: "vedio", Model: modelName, ChannelId: ch.id, Enabled: true, Priority: &priority, Weight: weight}).Error)
		}
	}
	model.InitChannelCache()
}

func distributeMiniMax(t *testing.T, tokenLimit map[string]bool, build func() (*http.Request, error)) (int, string, int) {
	t.Helper()
	selectedChannel, originalModel := 0, ""
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("id", 7)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "vedio")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "vedio")
		if tokenLimit != nil {
			common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
			common.SetContextKey(c, constant.ContextKeyTokenModelLimit, tokenLimit)
		}
	}, Distribute())
	handler := func(c *gin.Context) {
		selectedChannel = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		originalModel = c.GetString("original_model")
		c.Status(http.StatusOK)
	}
	engine.POST("/v1/files/upload", handler)
	engine.POST("/v1/music_generation", handler)
	req, err := build()
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder.Code, originalModel, selectedChannel
}

func miniMaxUploadForm(purpose string) func() (*http.Request, error) {
	return func() (*http.Request, error) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("model", "minimax-files-upload"); err != nil {
			return nil, err
		}
		if err := writer.WriteField("purpose", purpose); err != nil {
			return nil, err
		}
		part, err := writer.CreateFormFile("file", "input.bin")
		if err != nil {
			return nil, err
		}
		if _, err := part.Write([]byte("fixture")); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/files/upload", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		return req, nil
	}
}

func TestMiniMaxUploadRoutesByPurposeWithoutChangingBillingModel(t *testing.T) {
	setupMiniMaxUploadRouteChannels(t)
	tests := []struct {
		purpose     string
		wantChannel int
	}{
		{"voice_clone", 71},
		{"prompt_audio", 71},
		{"video_generation_input", 80},
		{"video_generation", 80},
		{"t2a_async_input", 80},
	}
	for _, tt := range tests {
		t.Run(tt.purpose, func(t *testing.T) {
			status, originalModel, channel := distributeMiniMax(t, nil, miniMaxUploadForm(tt.purpose))
			require.Equal(t, http.StatusOK, status)
			assert.Equal(t, tt.wantChannel, channel)
			assert.Equal(t, "minimax-files-upload", originalModel)
		})
	}
}

func TestMiniMaxUploadTokenLimitStillChecksUploadModel(t *testing.T) {
	setupMiniMaxUploadRouteChannels(t)
	status, _, channel := distributeMiniMax(t, map[string]bool{"minimax-files-upload": true}, miniMaxUploadForm("voice_clone"))
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, 71, channel)

	// The routing model grants no access by itself.
	status, _, _ = distributeMiniMax(t, map[string]bool{"minimax-voice-clone": true}, miniMaxUploadForm("voice_clone"))
	assert.Equal(t, http.StatusForbidden, status)
}

func TestMiniMaxMusicRequiresExplicitModel(t *testing.T) {
	setupMiniMaxUploadRouteChannels(t)
	status, _, channel := distributeMiniMax(t, nil, func() (*http.Request, error) {
		req := httptest.NewRequest(http.MethodPost, "/v1/music_generation", strings.NewReader(`{"prompt":"piano"}`))
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Zero(t, channel)
}
