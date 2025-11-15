package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	irc "github.com/thoj/go-ircevent"
	anthropicmodel "github.com/r33drichards/irc-agent/model/anthropic"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"
)

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
func NewIRCAgent(ctx context.Context, urlShortener *URLShortener) (*IRCAgent, error) {
	// Get environment variables
	server := os.Getenv("SERVER")
	channel := os.Getenv("CHANNEL")
	apiKey := os.Getenv("ANTHROPIC_API_KEY")

	if server == "" || channel == "" {
		return nil, fmt.Errorf("SERVER and CHANNEL environment variables are required")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is required")
	}

	// Create IRC connection
	ircConn := irc.IRC("agent", "agent")
	ircConn.UseTLS = false

	// Create Anthropic model (Claude 3.5 Haiku)
	model, err := anthropicmodel.NewModel(ctx, "claude-haiku-4-5", apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create model: %w", err)
	}

	// Create IRC message handler
	ircHandler := &IRCMessageHandler{
		conn: ircConn,
	}

	// Create TypeScript executor
	tsExecutor := &TypeScriptExecutor{
		SendMessage:  ircHandler.SendMessage,
		Channel:      channel,
		URLShortener: urlShortener,
	}

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

CRITICAL - Your Code Execution Capabilities:
You have the execute_typescript tool which gives you POWERFUL capabilities to accomplish virtually ANY task users request.
- NEVER say "I can't do that" or claim limitations when a task can be accomplished with code
- If a user asks you to do something (rename files, manipulate data, interact with APIs, process content, etc.), WRITE CODE to do it
- Don't ask for permission or additional tools - you already have the necessary permissions
- Be proactive and write the code needed to accomplish the user's goals
- If something doesn't exist (a function, API wrapper, etc.), write the code to create it yourself

IMPORTANT - Code Execution Results Workflow:
1. When you use execute_typescript, results are AUTOMATICALLY uploaded to S3
2. The response includes a "signed_url" field (OUTPUT, not input) with the URL to FULL results
3. The "output" field may be TRUNCATED (max 500 chars) to save tokens
4. If truncated, use execute_typescript again with Deno to download and inspect the full results
5. Signed URLs are valid for 24 hours

Note: signed_url is an OUTPUT field, NOT an input parameter to execute_typescript.

Deno Environment & Permissions:
- Deno runs with: --allow-env="AWS_*", --allow-net=s3.us-west-2.amazonaws.com,robust-cicada.s3.us-west-2.amazonaws.com,localhost:3000, --allow-read=., --allow-write=.
- AWS credentials are available via environment variables
- Full access to S3 bucket: s3://robust-cicada
- AWS SDK is available for Deno
- You can use npm packages with "npm:" prefix (e.g., "npm:@aws-sdk/client-s3@3")

URL Shortening Service:
- A URL shortener is running at http://localhost:3000
- Use POST requests to shorten long URLs (especially AWS S3 signed/presigned URLs)
- IMPORTANT: When users need to access URLs (especially signed URLs from S3), ALWAYS shorten them first
- This makes URLs much easier to copy, paste, and share in IRC
- Example use cases: S3 presigned URLs, API endpoints, any long URL a user might need

Example: Shorten a URL using fetch in Deno:
const longUrl = "https://robust-cicada.s3.us-west-2.amazonaws.com/...very-long-signed-url...";
const response = await fetch("http://localhost:3000/", {
  method: "POST",
  body: longUrl
});
const shortUrl = await response.text();
console.log("Short URL:", shortUrl);

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

Example: Rename an S3 object (copy then delete):
import { S3Client, CopyObjectCommand, DeleteObjectCommand } from "npm:@aws-sdk/client-s3@3";
const client = new S3Client({ region: "us-west-2" });
const oldKey = "1719040270770.jpeg";
const newKey = "hdsht.jpeg";
// Copy to new name
await client.send(new CopyObjectCommand({
  Bucket: "robust-cicada",
  CopySource: "robust-cicada/" + oldKey,
  Key: newKey
}));
// Delete old object
await client.send(new DeleteObjectCommand({
  Bucket: "robust-cicada",
  Key: oldKey
}));
console.log("Renamed " + oldKey + " to " + newKey);
`, channel),
		Tools: []tool.Tool{
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
	prompt := fmt.Sprintf("User %s in channel %s said: %s\n", sender, channel, message)

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

					// For non-IRC tools, show completion
					if toolName != "send_irc_message" {
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
