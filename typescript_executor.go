package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"google.golang.org/adk/tool"
)

// ExecuteTypeScriptParams defines the input parameters for executing TypeScript/JavaScript code
type ExecuteTypeScriptParams struct {
	Code string `json:"code" jsonschema:"The TypeScript or JavaScript code to execute"`
}

// ExecuteTypeScriptResults defines the output of TypeScript/JavaScript execution
type ExecuteTypeScriptResults struct {
	Status       string `json:"status"`
	Output       string `json:"output"`
	ErrorMessage string `json:"error_message,omitempty"`
	ExitCode     int    `json:"exit_code"`
	SignedURL    string `json:"signed_url,omitempty"`
	ShortURL     string `json:"short_url,omitempty"`
	CodeShortURL string `json:"code_short_url,omitempty"`
}

// TypeScriptExecutor handles TypeScript/JavaScript code execution using Deno
type TypeScriptExecutor struct {
	mu           sync.Mutex
	URLShortener *URLShortener
}

// uploadToS3AndGetSignedURL uploads content to S3 and returns a presigned URL
func uploadToS3AndGetSignedURL(ctx context.Context, content string) (string, error) {
	const bucketName = "robust-cicada"
	const region = "us-west-2"

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(cfg)

	// Generate a unique key based on timestamp and content hash
	hash := sha256.Sum256([]byte(content))
	hashStr := hex.EncodeToString(hash[:])[:16]
	timestamp := time.Now().Unix()
	key := fmt.Sprintf("code-results/%d-%s.txt", timestamp, hashStr)

	// Upload content to S3
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader([]byte(content)),
		ContentType: aws.String("text/plain"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Create S3 presign client
	presignClient := s3.NewPresignClient(s3Client)

	// Generate presigned URL (valid for 24 hours)
	presignResult, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(24*time.Hour))

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignResult.URL, nil
}

// Execute runs TypeScript/JavaScript code using Deno
func (e *TypeScriptExecutor) Execute(ctx tool.Context, params ExecuteTypeScriptParams) ExecuteTypeScriptResults {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Create a temporary directory for script isolation
	tempDir, err := os.MkdirTemp("", "deno-exec-")
	if err != nil {
		return ExecuteTypeScriptResults{
			Status:       "error",
			ErrorMessage: fmt.Sprintf("Failed to create temp directory: %v", err),
			ExitCode:     -1,
		}
	}
	defer os.RemoveAll(tempDir) // Clean up

	// Write the code to a temporary file
	scriptPath := filepath.Join(tempDir, "script.ts")
	err = os.WriteFile(scriptPath, []byte(params.Code), 0600)
	if err != nil {
		return ExecuteTypeScriptResults{
			Status:       "error",
			ErrorMessage: fmt.Sprintf("Failed to write script file: %v", err),
			ExitCode:     -1,
		}
	}

	// Upload code to S3 and get signed URL
	codeSignedURL, err := uploadToS3AndGetSignedURL(context.Background(), params.Code)
	var codeShortURL string
	if err != nil {
		log.Printf("Warning: Failed to upload code to S3: %v", err)
	} else if e.URLShortener != nil {
		codeShortURL = e.URLShortener.GetShortURL(codeSignedURL)
	}

	// Execute the script using Deno
	cmd := exec.Command(
		"deno",
		"run",
		"--no-check",
		"--allow-env=AWS_*,HOME,USERPROFILE,HOMEPATH,HOMEDRIVE,_X_AMZN_TRACE_ID",
		"--allow-net=s3.us-west-2.amazonaws.com,robust-cicada.s3.us-west-2.amazonaws.com,localhost:3000",
		"--allow-sys=osRelease",
		"--allow-read=.,/root/.cache/deno",
		"--allow-write=.",
		scriptPath,
	)
	cmd.Dir = tempDir

	// Capture stdout and stderr
	output, execErr := cmd.CombinedOutput()
	if execErr != nil {
		// command can exit with non-zero code and that would be
		// an error technically, but not an error logically
		log.Printf("Deno execution error: %v", execErr)
	}
	outputText := string(output)

	// Upload full result to S3 and get signed URL
	signedURL, uploadErr := uploadToS3AndGetSignedURL(context.Background(), outputText)
	if uploadErr != nil {
		log.Printf("Warning: Failed to upload result to S3: %v", uploadErr)
		// Continue without signed URL - don't fail the execution
		signedURL = ""
	}
	if execErr != nil {
		// Check if it's an exit error
		if exitErr, ok := execErr.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()

			// Check for permission errors
			if strings.Contains(outputText, "PermissionDenied") || strings.Contains(outputText, "permission denied") {
				return ExecuteTypeScriptResults{
					Status:       "error",
					Output:       outputText,
					ErrorMessage: "Permission denied. The server is configured with --allow-all, but the code may have additional permission requirements.",
					ExitCode:     exitCode,
				}
			}

			return ExecuteTypeScriptResults{
				Status:       "error",
				Output:       outputText,
				ErrorMessage: fmt.Sprintf("Execution failed with exit code %d", exitCode),
				ExitCode:     exitCode,
			}
		}

		// Other execution errors (e.g., Deno not found)
		return ExecuteTypeScriptResults{
			Status:       "error",
			Output:       outputText,
			ErrorMessage: fmt.Sprintf("Execution error: %v", execErr),
			ExitCode:     -1,
		}
	}

	// Successful execution
	fullResult := outputText
	if fullResult == "" {
		fullResult = "Code executed successfully (no output)"
	}

	// Truncate output if it's too large to avoid sending excessive tokens to LLM
	// Full output is always available via the signed URL
	const maxOutputLen = 500
	truncatedOutput := fullResult
	if len(fullResult) > maxOutputLen {
		truncatedOutput = fullResult[:maxOutputLen] + fmt.Sprintf("\n... (output truncated, %d more bytes available via signed_url)", len(fullResult)-maxOutputLen)
	}

	// Create shortened URL if we have a signed URL
	var shortURL string
	if signedURL != "" && e.URLShortener != nil {
		shortURL = e.URLShortener.GetShortURL(signedURL)
	}

	return ExecuteTypeScriptResults{
		Status:       "success",
		Output:       truncatedOutput,
		ExitCode:     0,
		SignedURL:    signedURL,
		ShortURL:     shortURL,
		CodeShortURL: codeShortURL,
	}
}
