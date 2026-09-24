package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskEndpointOverrideValidation(t *testing.T) {
	valid := []*TaskEndpointOverride{nil, {}, {SubmitPath: "/v1/services/aigc/video-generation/video-synthesis", FetchPath: "/api/v1/tasks/{task_id}"}, {FetchPath: "/tasks?id={task_id}"}}
	for _, override := range valid {
		require.NoError(t, override.Validate())
	}
	for _, tc := range []struct {
		path  string
		fetch bool
	}{
		{"https://other.example/tasks", false}, {"//other.example/tasks", false},
		{"/../tasks", false}, {"/%2e%2e/tasks", false}, {"/tasks#fragment", false},
		{" /tasks", false}, {"/tasks/{task_id}", false}, {"/tasks/{id}", true},
		{"/tasks", true}, {"/tasks/{task_id}/{task_id}", true}, {"/tasks?x=%zz", false},
	} {
		t.Run(tc.path, func(t *testing.T) { assert.Error(t, ValidateTaskEndpointPath(tc.path, tc.fetch)) })
	}
}
