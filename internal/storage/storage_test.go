package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock S3 ---

type mockS3 struct {
	putObjectFn func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	getObjectFn func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	putCalls    []s3.PutObjectInput
	getCalls    []s3.GetObjectInput
}

func (m *mockS3) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.putCalls = append(m.putCalls, *params)
	if m.putObjectFn != nil {
		return m.putObjectFn(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	m.getCalls = append(m.getCalls, *params)
	if m.getObjectFn != nil {
		return m.getObjectFn(ctx, params, optFns...)
	}
	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte("{}"))),
	}, nil
}

// --- Tests ---

func TestS3Key(t *testing.T) {
	t.Parallel()

	tests := []struct {
		project  string
		runID    string
		filename string
		expected string
	}{
		{"my-app", "abc12345", "stdout.log", "my-app/abc12345/stdout.log"},
		{"my-app", "abc12345", "meta.json", "my-app/abc12345/meta.json"},
		{"my-app", "abc12345", "exit_code", "my-app/abc12345/exit_code"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, S3Key(tt.project, tt.runID, tt.filename))
		})
	}
}

func TestNewStore(t *testing.T) {
	t.Parallel()

	s3api := &mockS3{}
	store := NewStore(s3api, "yeager-123")
	assert.NotNil(t, store)
	assert.Equal(t, "yeager-123", store.bucket)
}

func TestUploadOutput_Success(t *testing.T) {
	t.Parallel()

	s3api := &mockS3{}
	store := NewStore(s3api, "yeager-123")

	meta := RunMeta{
		RunID:     "abc12345",
		Command:   "cargo test",
		Project:   "my-app",
		ExitCode:  0,
		StartTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 1, 1, 0, 1, 23, 0, time.UTC),
		Duration:  "1m23s",
	}

	err := store.UploadOutput(
		context.Background(),
		"my-app", "abc12345",
		[]byte("test output"),
		[]byte("some warnings"),
		meta,
	)
	require.NoError(t, err)

	// Should have uploaded 4 objects: stdout, stderr, exit_code, meta.json
	require.Len(t, s3api.putCalls, 4)

	// Build a map of key → call for verification.
	byKey := make(map[string]s3.PutObjectInput)
	for _, call := range s3api.putCalls {
		byKey[*call.Key] = call
		assert.Equal(t, "yeager-123", *call.Bucket)
	}

	// Verify all expected keys are present.
	assert.Contains(t, byKey, "my-app/abc12345/stdout.log")
	assert.Contains(t, byKey, "my-app/abc12345/stderr.log")
	assert.Contains(t, byKey, "my-app/abc12345/exit_code")
	assert.Contains(t, byKey, "my-app/abc12345/meta.json")

	// Verify content types.
	for key, call := range byKey {
		if key == "my-app/abc12345/meta.json" {
			assert.Equal(t, "application/json", *call.ContentType)
		} else {
			assert.Equal(t, "text/plain", *call.ContentType, "expected text/plain for %s", key)
		}
	}

	// Verify body content for stdout.
	if call, ok := byKey["my-app/abc12345/stdout.log"]; ok {
		body, err := io.ReadAll(call.Body)
		require.NoError(t, err)
		assert.Equal(t, "test output", string(body))
	}

	// Verify body content for stderr.
	if call, ok := byKey["my-app/abc12345/stderr.log"]; ok {
		body, err := io.ReadAll(call.Body)
		require.NoError(t, err)
		assert.Equal(t, "some warnings", string(body))
	}

	// Verify body content for exit_code.
	if call, ok := byKey["my-app/abc12345/exit_code"]; ok {
		body, err := io.ReadAll(call.Body)
		require.NoError(t, err)
		assert.Equal(t, "0", string(body))
	}

	// Verify body content for meta.json.
	if call, ok := byKey["my-app/abc12345/meta.json"]; ok {
		body, err := io.ReadAll(call.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"run_id": "abc12345"`)
		assert.Contains(t, string(body), `"command": "cargo test"`)
		assert.Contains(t, string(body), `"exit_code": 0`)
	}
}

func TestUploadOutput_PutObjectError(t *testing.T) {
	t.Parallel()

	s3api := &mockS3{
		putObjectFn: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}
	store := NewStore(s3api, "yeager-123")

	err := store.UploadOutput(
		context.Background(),
		"my-app", "abc12345",
		[]byte("output"), []byte("errors"),
		RunMeta{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestDownloadOutput_Success(t *testing.T) {
	t.Parallel()

	s3api := &mockS3{
		getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			assert.Equal(t, "my-app/abc12345/stdout.log", *params.Key)
			return &s3.GetObjectOutput{
				Body: io.NopCloser(bytes.NewReader([]byte("the output data"))),
			}, nil
		},
	}
	store := NewStore(s3api, "yeager-123")

	data, err := store.DownloadOutput(context.Background(), "my-app", "abc12345")
	require.NoError(t, err)
	assert.Equal(t, "the output data", string(data))
}

func TestDownloadOutput_Error(t *testing.T) {
	t.Parallel()

	s3api := &mockS3{
		getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, fmt.Errorf("NoSuchKey")
		},
	}
	store := NewStore(s3api, "yeager-123")

	_, err := store.DownloadOutput(context.Background(), "my-app", "abc12345")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NoSuchKey")
}

func TestDownloadMeta_Success(t *testing.T) {
	t.Parallel()

	metaJSON := `{
		"run_id": "abc12345",
		"command": "cargo test",
		"project": "my-app",
		"exit_code": 1,
		"start_time": "2026-01-01T00:00:00Z",
		"end_time": "2026-01-01T00:01:23Z",
		"duration": "1m23s"
	}`

	s3api := &mockS3{
		getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			assert.Equal(t, "my-app/abc12345/meta.json", *params.Key)
			return &s3.GetObjectOutput{
				Body: io.NopCloser(bytes.NewReader([]byte(metaJSON))),
			}, nil
		},
	}
	store := NewStore(s3api, "yeager-123")

	meta, err := store.DownloadMeta(context.Background(), "my-app", "abc12345")
	require.NoError(t, err)
	assert.Equal(t, "abc12345", meta.RunID)
	assert.Equal(t, "cargo test", meta.Command)
	assert.Equal(t, 1, meta.ExitCode)
}

func TestDownloadMeta_InvalidJSON(t *testing.T) {
	t.Parallel()

	s3api := &mockS3{
		getObjectFn: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body: io.NopCloser(bytes.NewReader([]byte("not json"))),
			}, nil
		},
	}
	store := NewStore(s3api, "yeager-123")

	_, err := store.DownloadMeta(context.Background(), "my-app", "abc12345")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing meta.json")
}

func TestUploadArtifact_Success(t *testing.T) {
	t.Parallel()

	s3api := &mockS3{}
	store := NewStore(s3api, "yeager-123")

	err := store.UploadArtifact(
		context.Background(),
		"my-app", "abc12345",
		"coverage/report.html",
		[]byte("<html>coverage</html>"),
	)
	require.NoError(t, err)

	require.Len(t, s3api.putCalls, 1)
	assert.Equal(t, "my-app/abc12345/artifacts/coverage/report.html", *s3api.putCalls[0].Key)
	assert.Equal(t, "application/octet-stream", *s3api.putCalls[0].ContentType)
}

func TestUploadArtifact_StripsLeadingSlash(t *testing.T) {
	t.Parallel()

	s3api := &mockS3{}
	store := NewStore(s3api, "yeager-123")

	err := store.UploadArtifact(
		context.Background(),
		"my-app", "abc12345",
		"/coverage/report.html",
		[]byte("data"),
	)
	require.NoError(t, err)

	require.Len(t, s3api.putCalls, 1)
	assert.Equal(t, "my-app/abc12345/artifacts/coverage/report.html", *s3api.putCalls[0].Key)
}

func TestUploadArtifact_Error(t *testing.T) {
	t.Parallel()

	s3api := &mockS3{
		putObjectFn: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			return nil, fmt.Errorf("upload failed")
		},
	}
	store := NewStore(s3api, "yeager-123")

	err := store.UploadArtifact(
		context.Background(),
		"my-app", "abc12345",
		"report.html",
		[]byte("data"),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload failed")
}

func TestUploadArtifact_PathTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"simple relative traversal", "../secret.txt", true},
		{"deep traversal", "../../../etc/passwd", true},
		{"traversal with directory", "subdir/../../secret.txt", true},
		{"dot-dot only", "..", true},
		{"normal path", "coverage/report.html", false},
		{"leading slash normal", "/coverage/report.html", false},
		{"nested normal", "a/b/c/file.txt", false},
		{"single file", "report.html", false},
		{"dot in filename", "report.v2.html", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s3api := &mockS3{}
			store := NewStore(s3api, "yeager-123")

			err := store.UploadArtifact(
				context.Background(),
				"my-app", "abc12345",
				tt.path, []byte("data"),
			)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "escapes artifacts directory")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
