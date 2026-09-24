package taskcommon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTaskEndpointPreservesBasePathAndEscapesID(t *testing.T) {
	actual, err := ResolveEndpointURL("https://provider.example/prefix/", "/tasks/{task_id}", map[string]string{"task_id": "a/b?c&d"})
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example/prefix/tasks/a%2Fb%3Fc&d", actual)
	actual, err = ResolveEndpointURL("https://provider.example/prefix", "/tasks?id={task_id}", map[string]string{"task_id": "a/b?c&d"})
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example/prefix/tasks?id=a%2Fb%3Fc%26d", actual)
	for _, tc := range []struct{ base, path string }{
		{"file:///etc", "/tasks"}, {"https://provider.example", "//other.example/tasks"},
		{"https://provider.example", "/../tasks"}, {"https://provider.example", ""},
	} {
		_, err := ResolveEndpointURL(tc.base, tc.path, nil)
		assert.Error(t, err)
	}
}
