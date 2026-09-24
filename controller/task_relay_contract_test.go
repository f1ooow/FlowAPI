package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskResponseIsPublishedOnlyAfterPersistence(t *testing.T) {
	for _, persisted := range []bool{true, false} {
		t.Run(map[bool]string{true: "accepted", false: "insert failed"}[persisted], func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			response := newTaskResponseBuffer(c.Writer)
			c.Writer = response
			c.JSON(http.StatusOK, gin.H{"task_id": "public-task"})
			c.Writer.WriteHeaderNow()
			c.Writer.Flush()
			assert.Empty(t, recorder.Body.String(), "provider response must remain private until task is durable")
			if !persisted {
				response.reset()
				c.JSON(http.StatusInternalServerError, gin.H{"code": "task_persistence_failed"})
			}
			response.flushToClient()
			before := recorder.Body.String()
			response.flushToClient()
			assert.Equal(t, before, recorder.Body.String())
			if persisted {
				assert.Equal(t, 200, recorder.Code)
				assert.JSONEq(t, `{"task_id":"public-task"}`, before)
			} else {
				assert.Equal(t, 500, recorder.Code)
				assert.NotContains(t, before, "public-task")
				assert.JSONEq(t, `{"code":"task_persistence_failed"}`, before)
			}
		})
	}
}

func TestTaskRetryDoesNotReplayAmbiguousPaidPOST(t *testing.T) {
	for _, tc := range []struct {
		name, path                 string
		status                     int
		dispatched, definite, want bool
	}{
		{"unaccepted rate limit", "/v1/videos", 429, false, false, true},
		{"paid transport ambiguous", "/v1/videos", 502, true, false, false},
		{"paid timeout ambiguous", "/v1/videos", 408, true, false, false},
		{"definite provider rejection", "/v1/videos", 429, true, true, true},
		{"Wan recovery never replays", "/api/v1/services/aigc/video-generation/video-synthesis", 429, true, true, false},
		{"invalid request", "/v1/videos", 400, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", tc.path, nil)
			c.Set("task_paid_dispatched", tc.dispatched)
			c.Set("task_submission_definite_failure", tc.definite)
			assert.Equal(t, tc.want, shouldRetryTaskRelay(c, 1, &dto.TaskError{StatusCode: tc.status}, 2))
			require.False(t, shouldRetryTaskRelay(c, 1, nil, 2))
		})
	}
}
