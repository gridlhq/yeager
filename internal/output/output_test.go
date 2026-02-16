package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode Mode
		msg  string
		want string
	}{
		{
			name: "text mode prints prefixed message",
			mode: ModeText,
			msg:  "project: my-app",
			want: "yeager | project: my-app\n",
		},
		{
			name: "quiet mode suppresses info",
			mode: ModeQuiet,
			msg:  "project: my-app",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			w := NewWithWriters(&buf, &bytes.Buffer{}, tt.mode)
			w.Info(tt.msg)
			assert.Equal(t, tt.want, buf.String())
		})
	}
}

func TestInfoJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeJSON)
	w.Info("project: my-app")

	var got map[string]string
	err := json.Unmarshal(buf.Bytes(), &got)
	require.NoError(t, err)
	assert.Equal(t, "info", got["type"])
	assert.Equal(t, "project: my-app", got["message"])
}

func TestInfof(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeText)
	w.Infof("VM size: %s (%d vCPU, %d GB)", "medium", 4, 8)
	assert.Equal(t, "yeager | VM size: medium (4 vCPU, 8 GB)\n", buf.String())
}

func TestError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    Mode
		msg     string
		fix     string
		wantOut string
		wantErr string
	}{
		{
			name:    "text mode with fix suggestion",
			mode:    ModeText,
			msg:     "no AWS credentials found",
			fix:     "run: aws configure",
			wantOut: "",
			wantErr: "yeager | error: no AWS credentials found\nyeager | run: aws configure\n",
		},
		{
			name:    "text mode without fix",
			mode:    ModeText,
			msg:     "VM not found",
			fix:     "",
			wantOut: "",
			wantErr: "yeager | error: VM not found\n",
		},
		{
			name:    "quiet mode still shows errors",
			mode:    ModeQuiet,
			msg:     "connection failed",
			fix:     "check your network",
			wantOut: "",
			wantErr: "yeager | error: connection failed\nyeager | check your network\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, errBuf bytes.Buffer
			w := NewWithWriters(&out, &errBuf, tt.mode)
			w.Error(tt.msg, tt.fix)
			assert.Equal(t, tt.wantOut, out.String())
			assert.Equal(t, tt.wantErr, errBuf.String())
		})
	}
}

func TestErrorJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeJSON)
	w.Error("no AWS credentials found", "run: aws configure")

	var got map[string]string
	err := json.Unmarshal(buf.Bytes(), &got)
	require.NoError(t, err)
	assert.Equal(t, "error", got["type"])
	assert.Equal(t, "no AWS credentials found", got["message"])
	assert.Equal(t, "run: aws configure", got["fix"])
}

func TestErrorJSONWithoutFix(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeJSON)
	w.Error("connection failed", "")

	var got map[string]string
	err := json.Unmarshal(buf.Bytes(), &got)
	require.NoError(t, err)
	assert.Equal(t, "error", got["type"])
	assert.Equal(t, "connection failed", got["message"])
	_, hasFix := got["fix"]
	assert.False(t, hasFix, "fix field should be absent when empty")
}

func TestSeparator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode Mode
		want string
	}{
		{
			name: "text mode prints separator",
			mode: ModeText,
			want: "yeager | ─────────────────────────────────────────────\n",
		},
		{
			name: "quiet mode suppresses separator",
			mode: ModeQuiet,
			want: "",
		},
		{
			name: "json mode suppresses separator",
			mode: ModeJSON,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			w := NewWithWriters(&buf, &bytes.Buffer{}, tt.mode)
			w.Separator()
			assert.Equal(t, tt.want, buf.String())
		})
	}
}

func TestStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode Mode
		data string
		want string
	}{
		{
			name: "text mode passes through raw data",
			mode: ModeText,
			data: "test result: ok. 42 passed\n",
			want: "test result: ok. 42 passed\n",
		},
		{
			name: "quiet mode still shows stream output",
			mode: ModeQuiet,
			data: "test result: ok. 42 passed\n",
			want: "test result: ok. 42 passed\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			w := NewWithWriters(&buf, &bytes.Buffer{}, tt.mode)
			w.Stream([]byte(tt.data))
			assert.Equal(t, tt.want, buf.String())
		})
	}
}

func TestStreamJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeJSON)
	w.Stream([]byte("test result: ok\n"))

	var got map[string]string
	err := json.Unmarshal(buf.Bytes(), &got)
	require.NoError(t, err)
	assert.Equal(t, "output", got["type"])
	assert.Equal(t, "test result: ok", got["message"])
}

func TestStreamLine(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeText)
	w.StreamLine("compiling my-app v0.1.0")
	assert.Equal(t, "compiling my-app v0.1.0\n", buf.String())
}

func TestStreamLineJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeJSON)
	w.StreamLine("compiling my-app v0.1.0")

	var got map[string]string
	err := json.Unmarshal(buf.Bytes(), &got)
	require.NoError(t, err)
	assert.Equal(t, "output", got["type"])
	assert.Equal(t, "compiling my-app v0.1.0", got["message"])
}

func TestFullOutputSequence(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeText)

	w.Info("project: my-rust-app")
	w.Info("VM running (i-0a1b2c3d)")
	w.Info("syncing 3 changed files...")
	w.Info("running: cargo test")
	w.Separator()
	w.StreamLine("")
	w.StreamLine("test result: ok. 42 passed; 0 failed; 0 ignored")
	w.StreamLine("")
	w.Separator()
	w.Info("done (exit 0) in 14s")

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 10)
	assert.Equal(t, "yeager | project: my-rust-app", lines[0])
	assert.Equal(t, "yeager | VM running (i-0a1b2c3d)", lines[1])
	assert.Equal(t, "yeager | syncing 3 changed files...", lines[2])
	assert.Equal(t, "yeager | running: cargo test", lines[3])
	assert.True(t, strings.HasPrefix(lines[4], "yeager | ─"))
	assert.Equal(t, "", lines[5])
	assert.Equal(t, "test result: ok. 42 passed; 0 failed; 0 ignored", lines[6])
	assert.Equal(t, "", lines[7])
	assert.True(t, strings.HasPrefix(lines[8], "yeager | ─"))
	assert.Equal(t, "yeager | done (exit 0) in 14s", lines[9])
}

func TestJSONTimestamp(t *testing.T) {
	t.Parallel()

	fixedTime := time.Date(2026, 2, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name    string
		emit    func(w *Writer)
		wantTS  string
		wantKey string
	}{
		{
			name:    "Info includes timestamp",
			emit:    func(w *Writer) { w.Info("hello") },
			wantTS:  "2026-02-15T10:30:00Z",
			wantKey: "info",
		},
		{
			name:    "Error includes timestamp",
			emit:    func(w *Writer) { w.Error("oops", "") },
			wantTS:  "2026-02-15T10:30:00Z",
			wantKey: "error",
		},
		{
			name:    "Stream includes timestamp",
			emit:    func(w *Writer) { w.Stream([]byte("data")) },
			wantTS:  "2026-02-15T10:30:00Z",
			wantKey: "output",
		},
		{
			name:    "StreamLine includes timestamp",
			emit:    func(w *Writer) { w.StreamLine("line") },
			wantTS:  "2026-02-15T10:30:00Z",
			wantKey: "output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			w := NewWithWriters(&buf, &buf, ModeJSON)
			w.SetClock(func() time.Time { return fixedTime })
			tt.emit(w)

			var got map[string]string
			err := json.Unmarshal(buf.Bytes(), &got)
			require.NoError(t, err)
			assert.Equal(t, tt.wantKey, got["type"])
			assert.Equal(t, tt.wantTS, got["timestamp"])
		})
	}
}

func TestJSONTimestampIsRFC3339(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeJSON)
	w.Info("test")

	var got map[string]string
	err := json.Unmarshal(buf.Bytes(), &got)
	require.NoError(t, err)

	ts := got["timestamp"]
	require.NotEmpty(t, ts, "timestamp field must be present")
	_, err = time.Parse(time.RFC3339, ts)
	assert.NoError(t, err, "timestamp must be valid RFC 3339")
}

func TestNew(t *testing.T) {
	t.Parallel()

	// Verify the production constructor doesn't panic and sets correct mode.
	w := New(ModeText)
	require.NotNil(t, w)
	assert.Equal(t, ModeText, w.mode)
	assert.NotNil(t, w.out)
	assert.NotNil(t, w.err)
	assert.NotNil(t, w.now)

	w2 := New(ModeJSON)
	assert.Equal(t, ModeJSON, w2.mode)

	w3 := New(ModeQuiet)
	assert.Equal(t, ModeQuiet, w3.mode)
}

func TestStreamJSON_StripsTrailingNewline(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeJSON)
	w.Stream([]byte("output with newline\n"))

	var got map[string]string
	err := json.Unmarshal(buf.Bytes(), &got)
	require.NoError(t, err)
	assert.Equal(t, "output", got["type"])
	assert.Equal(t, "output with newline", got["message"], "trailing newline should be stripped in JSON mode")
}

func TestInfof_JSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := NewWithWriters(&buf, &bytes.Buffer{}, ModeJSON)
	w.Infof("count: %d, name: %s", 42, "test")

	var got map[string]string
	err := json.Unmarshal(buf.Bytes(), &got)
	require.NoError(t, err)
	assert.Equal(t, "info", got["type"])
	assert.Equal(t, "count: 42, name: test", got["message"])
}
