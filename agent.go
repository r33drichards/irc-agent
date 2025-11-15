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
	"github.com/thoj/go-ircevent"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/cmd/launcher/adk"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/server/restapi/services"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"
)

// SendIRCMessageParams defines the input parameters for sending IRC messages
type SendIRCMessageParams struct {
	Message string `json:"message" jsonschema:"The message to send to the IRC channel"`
	Channel string `json:"channel" jsonschema:"The IRC channel to send the message to"`
}

// SendIRCMessageResults defines the output of sending IRC messages
type SendIRCMessageResults struct {
	Status       string `json:"status"`
	Message      string `json:"message"`
	Channel      string `json:"channel"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// IRCMessageHandler handles IRC message sending with connection management
type IRCMessageHandler struct {
	conn *irc.Connection
	mu   sync.Mutex
}

// SendMessage sends a message to the IRC channel
func (h *IRCMessageHandler) SendMessage(ctx tool.Context, params SendIRCMessageParams) SendIRCMessageResults {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.conn == nil {
		return SendIRCMessageResults{
			Status:       "error",
			ErrorMessage: "IRC connection not initialized",
		}
	}

	// Send the message to the specified channel
	h.conn.Privmsg(params.Channel, params.Message)

	return SendIRCMessageResults{
		Status:  "success",
		Message: params.Message,
		Channel: params.Channel,
	}
}

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
}

// TypeScriptExecutor handles TypeScript/JavaScript code execution using Deno
type TypeScriptExecutor struct {
	mu sync.Mutex
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

	// Execute the script using Deno
	cmd := exec.Command(
		"deno",
		"run",
		"--no-check",
		"--allow-env=\"AWS_*\"",
		"--allow-net=s3.us-west-2.amazonaws.com,robust-cicada.s3.us-west-2.amazonaws.com",
		"--allow-read=.",
		"--allow-write=.",
		scriptPath,
	)
	cmd.Dir = tempDir

	// Capture stdout and stderr
	output, err := cmd.CombinedOutput()
	outputText := string(output)

	if err != nil {
		// Check if it's an exit error
		if exitErr, ok := err.(*exec.ExitError); ok {
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
			ErrorMessage: fmt.Sprintf("Execution error: %v", err),
			ExitCode:     -1,
		}
	}

	// Successful execution
	fullResult := outputText
	if fullResult == "" {
		fullResult = "Code executed successfully (no output)"
	}

	// Upload full result to S3 and get signed URL
	signedURL, err := uploadToS3AndGetSignedURL(context.Background(), fullResult)
	if err != nil {
		log.Printf("Warning: Failed to upload result to S3: %v", err)
		// Continue without signed URL - don't fail the execution
		return ExecuteTypeScriptResults{
			Status:   "success",
			Output:   fullResult,
			ExitCode: 0,
		}
	}

	// Truncate output if it's too large to avoid sending excessive tokens to LLM
	// Full output is always available via the signed URL
	const maxOutputLen = 500
	truncatedOutput := fullResult
	if len(fullResult) > maxOutputLen {
		truncatedOutput = fullResult[:maxOutputLen] + fmt.Sprintf("\n... (output truncated, %d more bytes available via signed_url)", len(fullResult)-maxOutputLen)
	}

	return ExecuteTypeScriptResults{
		Status:    "success",
		Output:    truncatedOutput,
		ExitCode:  0,
		SignedURL: signedURL,
	}
}

// IRCAgent wraps the ADK agent with IRC functionality
type IRCAgent struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	ircConn        *irc.Connection
	channel        string
	handler        *IRCMessageHandler
}

// NewIRCAgent creates a new IRC agent with ADK integration
func NewIRCAgent(ctx context.Context) (*IRCAgent, error) {
	// Get environment variables
	server := os.Getenv("SERVER")
	channel := os.Getenv("CHANNEL")
	apiKey := os.Getenv("GOOGLE_API_KEY")

	if server == "" || channel == "" {
		return nil, fmt.Errorf("SERVER and CHANNEL environment variables are required")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY environment variable is required")
	}

	// Create IRC connection
	ircConn := irc.IRC("agent", "agent")
	ircConn.UseTLS = false

	// Create Gemini model
	model, err := gemini.NewModel(ctx, "gemini-2.5-flash-lite", &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create model: %w", err)
	}

	// Create IRC message handler
	ircHandler := &IRCMessageHandler{
		conn: ircConn,
	}

	// Create IRC message tool using functiontool
	ircTool, err := functiontool.New(
		functiontool.Config{
			Name:        "send_irc_message",
			Description: "Sends a message to the IRC channel. Use this tool to respond to users in the IRC channel.",
		},
		ircHandler.SendMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create IRC tool: %w", err)
	}

	// Create TypeScript executor
	tsExecutor := &TypeScriptExecutor{}

	// Create TypeScript execution tool using functiontool
	tsTool, err := functiontool.New(
		functiontool.Config{
			Name:        "execute_typescript",
			Description: "Executes TypeScript/JavaScript code using Deno. Use this tool to help users with programming tasks or calculations. I will provide the necessary permissions for you to accomplish tasks",
		},
		tsExecutor.Execute,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create TypeScript execution tool: %w", err)
	}

	// Create ADK agent
	agent, err := llmagent.New(llmagent.Config{
		Name:        "irc_agent",
		Model:       model,
		Description: "An intelligent IRC bot that listens to messages and responds to users in the IRC channel.",
		Instruction: fmt.Sprintf(`You are a helpful IRC bot in the %s channel.
Your role is to assist users with their questions and engage in friendly conversation.
When users ask you questions or mention you, provide helpful and concise responses.
Your responses are automatically sent to the IRC channel, so just respond naturally.
Keep your responses brief and appropriate for IRC chat (usually 1-2 lines).
You have access to tools that will be displayed to users when used.
You can execute TypeScript/JavaScript code using the execute_typescript tool to help users with programming tasks or calculations.

IMPORTANT - Code Execution Results Workflow:
1. When you use execute_typescript, results are AUTOMATICALLY uploaded to S3
2. The response includes a "signed_url" field (OUTPUT, not input) with the URL to FULL results
3. The "output" field may be TRUNCATED (max 500 chars) to save tokens
4. If truncated, use execute_typescript again with Deno to download and inspect the full results
5. Signed URLs are valid for 24 hours

Note: signed_url is an OUTPUT field, NOT an input parameter to execute_typescript.

Deno Environment & Permissions:
- Deno runs with: --allow-env="AWS_*", --allow-net=s3.us-west-2.amazonaws.com,robust-cicada.s3.us-west-2.amazonaws.com, --allow-read=., --allow-write=.
- AWS credentials are available via environment variables
- Full access to S3 bucket: s3://robust-cicada
- AWS SDK is available for Deno

Example: Download file from signed URL using Deno:
const response = await fetch("SIGNED_URL_HERE");
const text = await response.text();
await Deno.writeTextFile("./result.txt", text);
const content = await Deno.readTextFile("./result.txt");
console.log(content);

Example: Use AWS SDK in Deno to interact with S3:
import { S3Client, GetObjectCommand } from "npm:@aws-sdk/client-s3@3";
const client = new S3Client({ region: "us-west-2" });
const command = new GetObjectCommand({
  Bucket: "robust-cicada",
  Key: "code-results/1234567890-abcdef.txt"
});
const response = await client.send(command);
const body = await response.Body.transformToString();
console.log(body);

Example: List all objects in an S3 bucket:
import { S3Client, ListObjectsV2Command } from "npm:@aws-sdk/client-s3@3";
const client = new S3Client({ region: "us-west-2" });
const command = new ListObjectsV2Command({
  Bucket: "robust-cicada"
});
const response = await client.send(command);
console.log(JSON.stringify(response.Contents, null, 2));

`, channel),
		Tools: []tool.Tool{
			ircTool,
			tsTool,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// Create session service
	sessionService := session.InMemoryService()

	// Create runner with in-memory services
	agentRunner, err := runner.New(runner.Config{
		AppName:         "irc_agent",
		Agent:           agent,
		SessionService:  sessionService,
		ArtifactService: artifact.InMemoryService(),
		MemoryService:   memory.InMemoryService(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create runner: %w", err)
	}

	return &IRCAgent{
		agent:          agent,
		runner:         agentRunner,
		sessionService: sessionService,
		ircConn:        ircConn,
		channel:        channel,
		handler:        ircHandler,
	}, nil
}

// Start connects to IRC and starts listening for messages
func (ia *IRCAgent) Start(ctx context.Context) error {
	server := os.Getenv("SERVER")

	// Set up IRC event handlers
	ia.ircConn.AddCallback("001", func(e *irc.Event) {
		log.Printf("Connected to IRC server")
		ia.ircConn.Join("#agent")
		log.Printf("Joined channel: #agent")
	})

	// Handle PRIVMSG events
	ia.ircConn.AddCallback("PRIVMSG", func(e *irc.Event) {
		message := e.Message()
		sender := e.Nick
		// Extract the channel from the event (first argument)
		channel := e.Arguments[0]

		log.Printf("[%s] <%s> %s", channel, sender, message)

		if e.Nick != "agent" {
			go ia.processMessage(ctx, sender, message, channel)
		}

	})

	// Connect to IRC server
	log.Printf("Connecting to IRC server: %s", server)
	err := ia.ircConn.Connect(server)
	if err != nil {
		return fmt.Errorf("failed to connect to IRC: %w", err)
	}

	// Start IRC event loop
	ia.ircConn.Loop()
	return nil
}

// processMessage sends the IRC message to the ADK agent for processing
func (ia *IRCAgent) processMessage(ctx context.Context, sender, message, channel string) {
	// Handle IRC commands (messages starting with /)
	if strings.HasPrefix(message, "/") {
		ia.handleCommand(sender, message, channel)
		return
	}

	// Create a prompt for the agent that includes the channel context
	prompt := fmt.Sprintf("User %s in channel %s said: %s\n\nIMPORTANT: When responding, you MUST use the send_irc_message tool with channel parameter set to: %s", sender, channel, message, channel)

	log.Printf("Processing message from %s in %s: %s", sender, channel, message)

	// Create the content for the agent
	content := genai.NewContentFromText(prompt, genai.RoleUser)

	// Use a unique session ID for the channel to maintain conversation history
	sessionID := fmt.Sprintf("irc-session-%s", channel)

	// Ensure session exists - create it if it doesn't
	_, err := ia.sessionService.Get(ctx, &session.GetRequest{
		AppName:   "irc_agent",
		UserID:    channel,
		SessionID: sessionID,
	})
	if err != nil {
		// Session doesn't exist, create it
		log.Printf("Creating new session for channel %s", channel)
		_, err = ia.sessionService.Create(ctx, &session.CreateRequest{
			AppName:   "irc_agent",
			UserID:    channel,
			SessionID: sessionID,
			State:     make(map[string]any),
		})
		if err != nil {
			log.Printf("Error creating session: %v", err)
			return
		}
	}

	// Run the agent with the message
	runConfig := agent.RunConfig{}
	events := ia.runner.Run(ctx, channel, sessionID, content, runConfig)

	// Process the events
	for event, err := range events {
		if err != nil {
			log.Printf("Error processing message: %v", err)
			ia.ircConn.Privmsg(channel, fmt.Sprintf("Error: %v", err))
			return
		}

		// Process event content
		if event != nil && event.Content != nil && len(event.Content.Parts) > 0 {
			log.Printf("Agent event - Author: %s, InvocationID: %s", event.Author, event.InvocationID)

			for _, part := range event.Content.Parts {
				// Handle text responses - send directly to IRC
				if part.Text != "" && event.Author != genai.RoleUser {
					log.Printf("Agent text response: %s", part.Text)
					// Split long messages if needed (IRC has message length limits)
					ia.sendToIRC(part.Text, channel)
				}

				// Handle function calls - send summary to IRC
				if part.FunctionCall != nil {
					toolName := part.FunctionCall.Name
					log.Printf("Agent calling tool: %s", toolName)

					// Don't send notification for send_irc_message tool to avoid clutter
					if toolName != "send_irc_message" {
						summary := fmt.Sprintf("[Using tool: %s]", toolName)
						ia.ircConn.Privmsg(channel, summary)
					}
				}

				// Handle function responses - send summary for non-IRC tools
				if part.FunctionResponse != nil {
					toolName := part.FunctionResponse.Name
					log.Printf("Tool %s responded", toolName)

					// For execute_typescript, automatically send the signed URL if available
					if toolName == "execute_typescript" {
						if signedURL, ok := part.FunctionResponse.Response["signed_url"].(string); ok && signedURL != "" {
							urlMessage := fmt.Sprintf("Code execution results: %s", signedURL)
							ia.ircConn.Privmsg(channel, urlMessage)
						}
						summary := fmt.Sprintf("[Tool %s completed]", toolName)
						ia.ircConn.Privmsg(channel, summary)
					} else if toolName != "send_irc_message" {
						// For non-IRC tools, show completion
						summary := fmt.Sprintf("[Tool %s completed]", toolName)
						ia.ircConn.Privmsg(channel, summary)
					}
				}
			}

			// Log if this is a final response
			if event.IsFinalResponse() {
				log.Printf("Agent sent final response")
			}
		}
	}

	log.Printf("Agent finished processing message from %s in %s", sender, channel)
}

// handleCommand processes IRC commands sent to the agent
func (ia *IRCAgent) handleCommand(sender, message, sourceChannel string) {
	// Parse the command and arguments
	parts := strings.Fields(message)
	if len(parts) == 0 {
		return
	}

	command := strings.ToLower(parts[0])
	args := parts[1:]

	log.Printf("User %s sent command: %s %v", sender, command, args)

	switch command {
	case "/join":
		if len(args) < 1 {
			ia.ircConn.Privmsg(sourceChannel, fmt.Sprintf("%s: Usage: /join #channel", sender))
			return
		}
		channel := args[0]
		ia.ircConn.Join(channel)
		log.Printf("Joining channel %s (requested by %s)", channel, sender)
		ia.ircConn.Privmsg(sourceChannel, fmt.Sprintf("%s: Joining %s", sender, channel))

	case "/part":
		if len(args) < 1 {
			ia.ircConn.Privmsg(sourceChannel, fmt.Sprintf("%s: Usage: /part #channel", sender))
			return
		}
		channel := args[0]
		ia.ircConn.Part(channel)
		log.Printf("Leaving channel %s (requested by %s)", channel, sender)
		ia.ircConn.Privmsg(sourceChannel, fmt.Sprintf("%s: Leaving %s", sender, channel))

	case "/nick":
		if len(args) < 1 {
			ia.ircConn.Privmsg(sourceChannel, fmt.Sprintf("%s: Usage: /nick newnick", sender))
			return
		}
		newNick := args[0]
		ia.ircConn.Nick(newNick)
		log.Printf("Changing nick to %s (requested by %s)", newNick, sender)
		ia.ircConn.Privmsg(sourceChannel, fmt.Sprintf("%s: Changing nick to %s", sender, newNick))

	default:
		ia.ircConn.Privmsg(sourceChannel, fmt.Sprintf("%s: Unknown command: %s. Available commands: /join, /part, /nick", sender, command))
	}
}

// sendToIRC sends a message to IRC, splitting if necessary for length limits
func (ia *IRCAgent) sendToIRC(message, channel string) {
	// IRC message limit is typically around 512 bytes, but we'll use 400 to be safe
	const maxLen = 400

	if len(message) <= maxLen {
		ia.ircConn.Privmsg(channel, message)
		return
	}

	// Split long messages into chunks
	for len(message) > 0 {
		end := maxLen
		if end > len(message) {
			end = len(message)
		}

		// Try to break at a space if possible
		if end < len(message) {
			lastSpace := end
			for i := end - 1; i > end-50 && i > 0; i-- {
				if message[i] == ' ' {
					lastSpace = i
					break
				}
			}
			if lastSpace != end {
				end = lastSpace
			}
		}

		ia.ircConn.Privmsg(channel, message[:end])
		message = message[end:]
		if len(message) > 0 && message[0] == ' ' {
			message = message[1:] // Skip leading space
		}
	}
}

func main() {
	ctx := context.Background()

	// Create IRC Agent
	ircAgent, err := NewIRCAgent(ctx)
	if err != nil {
		log.Fatalf("Failed to create IRC agent: %v", err)
	}

	// Check if we should run in web mode or IRC mode
	if len(os.Args) > 1 && os.Args[1] == "web" {
		// Run with ADK web interface
		config := &adk.Config{
			AgentLoader: services.NewSingleAgentLoader(ircAgent.agent),
		}

		l := full.NewLauncher()
		err = l.Execute(ctx, config, os.Args[1:])
		if err != nil {
			log.Fatalf("Web launcher failed: %v\n\n%s", err, l.CommandLineSyntax())
		}
	} else {
		// Run in IRC mode
		log.Println("Starting IRC Agent...")
		log.Printf("Channel: %s", ircAgent.channel)

		if err := ircAgent.Start(ctx); err != nil {
			log.Fatalf("IRC agent failed: %v", err)
		}
	}
}
