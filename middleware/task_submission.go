package middleware

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// TaskSubmission reserves the client key before channel selection or paid dispatch.
func TaskSubmission() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path != dto.WanCreatePath {
			c.Next()
			return
		}
		c.Header("X-Keli-Task-Recovery", "request-id-v1")
		key := c.GetHeader("X-Keli-Submission-Key")
		if key == "" {
			c.Next()
			return
		}
		if !model.ValidTaskSubmissionKey(key) {
			c.AbortWithStatusJSON(400, gin.H{"code": "invalid_submission_key"})
			return
		}
		body, err := common.GetBodyStorage(c)
		if err != nil {
			c.AbortWithStatusJSON(400, gin.H{"code": "read_request_body_failed"})
			return
		}
		digest := sha256.New()
		_, _ = io.WriteString(digest, c.Request.URL.Path+"\n")
		if _, err = io.Copy(digest, body); err != nil {
			c.AbortWithStatusJSON(400, gin.H{"code": "read_request_body_failed"})
			return
		}
		if _, err = body.Seek(0, io.SeekStart); err != nil {
			c.AbortWithStatusJSON(500, gin.H{"code": "read_request_body_failed"})
			return
		}
		c.Request.Body = io.NopCloser(body)
		submission, owner, err := model.ReserveTaskSubmission(c.GetInt("id"), c.GetInt("token_id"), key, fmt.Sprintf("%x", digest.Sum(nil)))
		if errors.Is(err, model.ErrSubmissionConflict) {
			c.AbortWithStatusJSON(409, gin.H{"code": "idempotency_conflict"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(500, gin.H{"code": "submission_storage_failed"})
			return
		}
		if !owner {
			if submission.Status == "accepted" {
				c.AbortWithStatusJSON(200, gin.H{"output": gin.H{"task_id": submission.TaskID, "task_status": "PENDING"}})
				return
			}
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"code": "submission_pending", "status": submission.Status})
			return
		}
		c.Set("task_submission_id", submission.ID)
		c.Set("task_submission_public_id", submission.TaskID)
		// The client may leave while the provider is accepting a paid job. Finish and persist it.
		c.Request = c.Request.WithContext(context.WithoutCancel(c.Request.Context()))
		defer func() {
			if err := model.FinishTaskSubmission(submission.ID, c.GetBool("task_submission_dispatched"), c.GetBool("task_submission_definite_failure")); err != nil {
				common.SysError("finish task submission failed: " + err.Error())
			}
		}()
		c.Next()
	}
}
