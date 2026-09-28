package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWanProbeUsesVideoTrackNotAudioTail(t *testing.T) {
	box := func(name string, data []byte) []byte {
		b := make([]byte, 8)
		binary.BigEndian.PutUint32(b, uint32(8+len(data)))
		copy(b[4:], name)
		return append(b, data...)
	}
	track := func(handler string, duration uint32) []byte {
		mdhd := make([]byte, 24)
		binary.BigEndian.PutUint32(mdhd[12:], 1000)
		binary.BigEndian.PutUint32(mdhd[16:], duration)
		hdlr := make([]byte, 25)
		copy(hdlr[8:], handler)
		return box("trak", box("mdia", append(box("mdhd", mdhd), box("hdlr", hdlr)...)))
	}
	for _, tc := range []struct {
		name    string
		tracks  []byte
		seconds float64
		invalid bool
	}{
		{"video and audio tail", append(track("vide", 4000), track("soun", 4040)...), 4, false},
		{"audio only", track("soun", 4040), 0, true},
		{"absurd video duration", track("vide", 4000000), 0, true},
		{"empty media", nil, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seconds, err := wanVideoTrackDuration(bytes.NewReader(box("moov", tc.tracks)))
			if tc.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.seconds, seconds)
			}
		})
	}
}

func TestWanBillingUsesEffectiveSnapshotAndAuditsSaturation(t *testing.T) {
	effective := 2.4
	task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{BillingUnit: "output_seconds", ModelPrice: .6, GroupRatio: 99, EffectiveRatio: &effective, OutputVideoDuration: 4, OtherRatios: map[string]float64{"seconds": 30, "resolution": .5}}}}
	quota, clamp := wanActualQuotaChecked(task)
	require.Nil(t, clamp)
	assert.Equal(t, common.QuotaRound(.6*2.4*.5*4*common.QuotaPerUnit), quota)
	task.PrivateData.BillingContext.EffectiveRatio = nil
	task.PrivateData.BillingContext.GroupRatio = 1.2
	quota, clamp = wanActualQuotaChecked(task)
	require.Nil(t, clamp)
	assert.Equal(t, common.QuotaRound(.6*1.2*.5*4*common.QuotaPerUnit), quota)
	task.PrivateData.BillingContext.ModelPrice = math.MaxFloat64
	quota, clamp = wanActualQuotaChecked(task)
	require.NotNil(t, clamp)
	assert.Equal(t, common.MaxQuota, quota)
	task.PrivateData.BillingContext.ModelPrice = -1
	quota, clamp = wanActualQuotaChecked(task)
	require.NotNil(t, clamp)
	assert.Zero(t, quota)
	task.PrivateData.BillingContext.ModelPrice = .6
	task.PrivateData.BillingContext.OtherRatios = nil
	quota, clamp = wanActualQuotaChecked(task)
	require.Nil(t, clamp)
	assert.Equal(t, common.QuotaRound(.6*1.2*4*common.QuotaPerUnit), quota, "legacy snapshots without resolution use the base ratio")
	task.PrivateData.BillingContext.OtherRatios = map[string]float64{"resolution": math.NaN()}
	quota, clamp = wanActualQuotaChecked(task)
	require.NotNil(t, clamp)
	assert.Zero(t, quota)
	task.PrivateData.BillingContext.OtherRatios = map[string]float64{"resolution": 0}
	quota, clamp = wanActualQuotaChecked(task)
	require.NotNil(t, clamp)
	assert.Zero(t, quota)
}

type wanBillingTestAdaptor struct{ result *relaycommon.TaskInfo }

func (a *wanBillingTestAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *wanBillingTestAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"status":"completed"}`))}, nil
}
func (a *wanBillingTestAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	r := *a.result
	return &r, nil
}
func (a *wanBillingTestAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 999999999
}

func TestWanBillingSnapshotAndRepeatedTerminal(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "success", false: "failure"}[success], func(t *testing.T) {
			truncate(t)
			seedUser(t, 801, 10000000)
			seedToken(t, 802, 801, "wan-test-token", 10000000)
			seedChannel(t, 803)
			pre := common.QuotaRound(.6 * 1.2 * .5 * 5 * common.QuotaPerUnit)
			task := makeTask(801, 803, pre, 802, "wallet", 0)
			task.TaskID = "wan-billing"
			task.SubmitTime = time.Now().Unix()
			task.Properties.OriginModelName = "wan3.0-video"
			task.PrivateData.BillingContext = &model.TaskBillingContext{BillingUnit: "output_seconds", ModelPrice: .6, GroupRatio: 1.2, OtherRatios: map[string]float64{"seconds": 5, "resolution": .5}, OriginModelName: "wan3.0-video", PerCallBilling: true}
			require.NoError(t, task.Insert())
			var stale model.Task
			require.NoError(t, model.DB.First(&stale, task.ID).Error)
			result := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, Url: "https://example.com/video.mp4", OutputVideoDuration: 2, TotalTokens: 999999999}
			if !success {
				result.Status = model.TaskStatusFailure
				result.Reason = "upstream failed"
			}
			a := &wanBillingTestAdaptor{result: result}
			ch := &model.Channel{Id: 803, Type: 1, Key: "fixture"}
			require.NoError(t, updateVideoSingleTask(context.Background(), a, ch, task.TaskID, map[string]*model.Task{task.TaskID: task}))
			quota := getUserQuota(t, 801)
			expected := 10000000 + pre
			if success {
				expected -= common.QuotaRound(.6 * 1.2 * .5 * 2 * common.QuotaPerUnit)
				assert.Equal(t, 2.0, task.PrivateData.BillingContext.OutputVideoDuration)
			}
			assert.Equal(t, expected, quota)
			require.NoError(t, updateVideoSingleTask(context.Background(), a, ch, task.TaskID, map[string]*model.Task{task.TaskID: task}))
			require.NoError(t, updateVideoSingleTask(context.Background(), a, ch, stale.TaskID, map[string]*model.Task{stale.TaskID: &stale}))
			assert.Equal(t, quota, getUserQuota(t, 801))
			assert.Equal(t, quota, getTokenRemainQuota(t, 802))
		})
	}
}

func TestWanSettlementBillsOutputPlusReferenceSeconds(t *testing.T) {
	// 0.6 CNY/s × group 1.2 × 480P 0.5 × 500000 = 180000 quota per billed second.
	const perSecond = 180000
	tests := []struct {
		name           string
		requested      float64
		input          float64
		inputBilled    bool
		providerOutput float64
		wantQuota      int
		wantOutput     float64
		wantUnverified bool
	}{
		{name: "no reference video charges output only", requested: 5, providerOutput: 2, wantQuota: 2 * perSecond, wantOutput: 2},
		{name: "reference video adds its seconds", requested: 5, input: 5, inputBilled: true, providerOutput: 3, wantQuota: 8 * perSecond, wantOutput: 3},
		{name: "missing provider duration settles at requested output plus reference", requested: 5, input: 5, inputBilled: true, wantQuota: 10 * perSecond, wantOutput: 5, wantUnverified: true},
		{name: "provider duration over the 30s total settles at requested", requested: 5, input: 5, inputBilled: true, providerOutput: 28, wantQuota: 10 * perSecond, wantOutput: 5, wantUnverified: true},
		{name: "missing provider duration without reference settles at requested", requested: 5, wantQuota: 5 * perSecond, wantOutput: 5, wantUnverified: true},
		{name: "legacy snapshot does not bill reference seconds", requested: 5, input: 5, providerOutput: 5, wantQuota: 5 * perSecond, wantOutput: 5},
		{name: "legacy snapshot with missing duration keeps output-only reservation", requested: 5, input: 5, wantQuota: 5 * perSecond, wantOutput: 5, wantUnverified: true},
		{name: "longer provider output never charges above reservation", requested: 5, providerOutput: 9, wantQuota: 5 * perSecond, wantOutput: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 801, 10000000)
			seedToken(t, 802, 801, "wan-reference", 10000000)
			seedChannel(t, 803)
			reservedSeconds := tt.requested
			if tt.inputBilled {
				reservedSeconds += tt.input
			}
			pre := int(reservedSeconds) * perSecond
			task := makeTask(801, 803, pre, 802, "wallet", 0)
			task.TaskID = "wan-reference"
			task.SubmitTime = time.Now().Unix()
			task.Properties.OriginModelName = "wan3.0-video"
			task.PrivateData.BillingContext = &model.TaskBillingContext{
				BillingUnit: "output_seconds", ModelPrice: .6, GroupRatio: 1.2, InputVideoDuration: tt.input, WanInputBilled: tt.inputBilled,
				OtherRatios: map[string]float64{"seconds": reservedSeconds, "resolution": .5}, OriginModelName: "wan3.0-video",
			}
			require.NoError(t, task.Insert())
			a := &wanBillingTestAdaptor{result: &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, Url: "https://example.com/video.mp4", OutputVideoDuration: tt.providerOutput}}
			ch := &model.Channel{Id: 803, Type: 1, Key: "fixture"}
			for range 2 {
				require.NoError(t, updateVideoSingleTask(context.Background(), a, ch, task.TaskID, map[string]*model.Task{task.TaskID: task}))
			}

			var stored model.Task
			require.NoError(t, model.DB.First(&stored, task.ID).Error)
			assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), stored.Status)
			assert.Equal(t, "https://example.com/video.mp4", stored.GetResultURL())
			assert.Equal(t, tt.wantQuota, stored.Quota)
			assert.Equal(t, tt.wantOutput, stored.PrivateData.BillingContext.OutputVideoDuration)
			assert.Equal(t, tt.wantUnverified, stored.PrivateData.BillingContext.OutputDurationUnverified)
			assert.Equal(t, 10000000+pre-tt.wantQuota, getUserQuota(t, 801))
			assert.Equal(t, 10000000+pre-tt.wantQuota, getTokenRemainQuota(t, 802))
		})
	}
}

func TestWanBillingRejectsCorruptReferenceSeconds(t *testing.T) {
	for _, input := range []float64{-1, math.NaN(), 31} {
		task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{BillingUnit: "output_seconds", ModelPrice: .6, GroupRatio: 1, OutputVideoDuration: 4, InputVideoDuration: input, WanInputBilled: true, OtherRatios: map[string]float64{"resolution": 1}}}}
		quota, clamp := wanActualQuotaChecked(task)
		require.NotNil(t, clamp)
		assert.Zero(t, quota)
	}
}
