package git

import (
	"context"
	"testing"

	"dappco.re/go/core"
	"github.com/stretchr/testify/require"
)

const testFileMode = 0o644

func writeTestFile(t *testing.T, filename, content string) {
	t.Helper()

	result := core.New().Fs().WriteMode(filename, content, testFileMode)
	require.True(t, result.OK, "failed to write %s: %v", filename, result.Value)
}

func deleteTestPath(t *testing.T, target string) {
	t.Helper()

	result := core.New().Fs().Delete(target)
	require.True(t, result.OK, "failed to delete %s: %v", target, result.Value)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	_, err := gitCommand(context.Background(), dir, args...)
	require.NoError(t, err, "failed to run git %v", args)
}
