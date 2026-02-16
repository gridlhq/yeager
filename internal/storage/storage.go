package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3API is the subset of the S3 client used by storage operations.
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// RunMeta contains metadata about a completed run, stored as meta.json in S3.
type RunMeta struct {
	RunID     string    `json:"run_id"`
	Command   string    `json:"command"`
	Project   string    `json:"project"`
	ExitCode  int       `json:"exit_code"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  string    `json:"duration"`
}

// Store handles S3 output storage operations.
type Store struct {
	s3     S3API
	bucket string
}

// NewStore creates a Store with the given S3 client and bucket name.
func NewStore(s3api S3API, bucket string) *Store {
	return &Store{s3: s3api, bucket: bucket}
}

// S3Key returns the S3 key prefix for a project+run combination.
func S3Key(projectName, runID, filename string) string {
	return fmt.Sprintf("%s/%s/%s", projectName, runID, filename)
}

// UploadOutput uploads stdout, stderr, exit code, and metadata for a completed run.
func (s *Store) UploadOutput(ctx context.Context, projectName, runID string, stdout, stderr []byte, meta RunMeta) error {
	uploads := map[string][]byte{
		"stdout.log": stdout,
		"stderr.log": stderr,
	}

	// Upload stdout and stderr.
	for name, data := range uploads {
		key := S3Key(projectName, runID, name)
		if err := s.putObject(ctx, key, data, "text/plain"); err != nil {
			return fmt.Errorf("uploading %s: %w", name, err)
		}
	}

	// Upload exit code.
	exitKey := S3Key(projectName, runID, "exit_code")
	exitData := []byte(fmt.Sprintf("%d", meta.ExitCode))
	if err := s.putObject(ctx, exitKey, exitData, "text/plain"); err != nil {
		return fmt.Errorf("uploading exit_code: %w", err)
	}

	// Upload meta.json.
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling meta.json: %w", err)
	}
	metaKey := S3Key(projectName, runID, "meta.json")
	if err := s.putObject(ctx, metaKey, metaJSON, "application/json"); err != nil {
		return fmt.Errorf("uploading meta.json: %w", err)
	}

	slog.Debug("uploaded run output to S3",
		"bucket", s.bucket,
		"prefix", fmt.Sprintf("%s/%s/", projectName, runID),
	)
	return nil
}

// DownloadOutput retrieves the stdout log for a specific run from S3.
func (s *Store) DownloadOutput(ctx context.Context, projectName, runID string) ([]byte, error) {
	key := S3Key(projectName, runID, "stdout.log")
	return s.getObject(ctx, key)
}

// DownloadMeta retrieves the meta.json for a specific run from S3.
func (s *Store) DownloadMeta(ctx context.Context, projectName, runID string) (*RunMeta, error) {
	key := S3Key(projectName, runID, "meta.json")
	data, err := s.getObject(ctx, key)
	if err != nil {
		return nil, err
	}

	var meta RunMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing meta.json: %w", err)
	}
	return &meta, nil
}

// UploadArtifact uploads a single artifact file to S3.
func (s *Store) UploadArtifact(ctx context.Context, projectName, runID, artifactPath string, data []byte) error {
	// Normalize the artifact path: clean traversal sequences and strip leading slashes.
	cleaned := path.Clean(artifactPath)
	cleaned = strings.TrimPrefix(cleaned, "/")

	// Reject paths that escape the artifacts directory.
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("artifact path %q escapes artifacts directory", artifactPath)
	}

	key := fmt.Sprintf("%s/%s/artifacts/%s", projectName, runID, cleaned)
	return s.putObject(ctx, key, data, "application/octet-stream")
}

func (s *Store) putObject(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("putting object s3://%s/%s: %w", s.bucket, key, err)
	}
	return nil
}

func (s *Store) getObject(ctx context.Context, key string) ([]byte, error) {
	out, err := s.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("getting object s3://%s/%s: %w", s.bucket, key, err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("reading object s3://%s/%s: %w", s.bucket, key, err)
	}
	return data, nil
}
