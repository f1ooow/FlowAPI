package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetTaskSubmission(c *gin.Context) {
	key := c.Param("client_key")
	if !model.ValidTaskSubmissionKey(key) {
		c.JSON(400, gin.H{"code": "invalid_submission_key"})
		return
	}
	submission, err := model.GetTaskSubmission(c.GetInt("id"), c.GetInt("token_id"), key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(404, gin.H{"code": "submission_not_found"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"code": "submission_storage_failed"})
		return
	}
	taskID := ""
	if submission.Status == "accepted" {
		taskID = submission.TaskID
	}
	c.JSON(200, gin.H{"submission_key": submission.ClientKey, "task_id": taskID, "status": submission.Status})
}
