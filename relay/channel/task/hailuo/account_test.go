package hailuo

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/minimax"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestH3UploadedFileRequiresOriginalAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB; sqlDB.Close() })
	require.NoError(t, minimax.MigrateResourceBindings(db))
	info := &relaycommon.RelayInfo{UserId: 21, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 62, ChannelBaseUrl: "https://provider.example/minimax", ApiKey: "original-key", UpstreamModelName: ModelMiniMaxH3}}
	const fileID = "9223372036854775807"
	require.NoError(t, minimax.BindResource(info, "file", fileID))
	req := relaycommon.TaskSubmitReq{Prompt: "animate", Duration: 5, Metadata: map[string]any{"first_frame_image": "mm_file://" + fileID}}
	a := &TaskAdaptor{}
	payload, err := a.convertToRequestPayload(&req, info)
	require.NoError(t, err)
	assert.Equal(t, "mm_file://"+fileID, payload.(*H3VideoRequest).Content[1].ImageURL.URL)
	info.ApiKey = "rotated-other-account"
	_, err = a.convertToRequestPayload(&req, info)
	require.ErrorContains(t, err, "different channel or credential")
	info.ApiKey = "original-key"
	info.ChannelId = 49
	_, err = a.convertToRequestPayload(&req, info)
	require.Error(t, err)
	info.ChannelId = 62
	info.UserId = 22
	_, err = a.convertToRequestPayload(&req, info)
	require.Error(t, err)
}
