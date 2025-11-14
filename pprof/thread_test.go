package pprof

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThreadLabelsAdded(t *testing.T) {
	// Read example JFR file
	jfrFile := "../parser/testdata/example.jfr.gz"
	body, err := os.ReadFile(jfrFile)
	require.NoError(t, err)

	// Decompress if needed
	if len(body) > 2 && body[0] == 0x1f && body[1] == 0x8b {
		r := heapReader()
		body, _ = r(t, jfrFile)
	}

	// Initialize empty jfrLabels so thread labels can be added
	jfrLabels := &LabelsSnapshot{
		Contexts: make(map[int64]*Context),
		Strings:  make(map[int64]string),
	}

	// Parse JFR
	profiles, err := ParseJFR(body, parseInput, jfrLabels)
	require.NoError(t, err)
	require.NotNil(t, profiles)
	require.Greater(t, len(profiles.Profiles), 0, "Should have at least one profile")

	// Check if any samples have thread labels
	foundThreadId := false
	foundThreadName := false

	for _, profile := range profiles.Profiles {
		for _, sample := range profile.Profile.Sample {
			for _, label := range sample.Label {
				key := profile.Profile.StringTable[label.Key]
				if key == "thread_id" {
					foundThreadId = true
					assert.Greater(t, label.Num, int64(0), "thread_id should be > 0")
				}
				if key == "thread_name" {
					foundThreadName = true
					threadName := profile.Profile.StringTable[label.Str]
					assert.NotEmpty(t, threadName, "thread_name should not be empty")
					t.Logf("Found thread: name=%s", threadName)
				}
			}
		}
	}

	assert.True(t, foundThreadId, "At least one sample should have thread_id label")
	assert.True(t, foundThreadName, "At least one sample should have thread_name label")
}
