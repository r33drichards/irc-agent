package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"

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

// ExecuteDenoCodeParams defines the input parameters for executing Deno code
type ExecuteDenoCodeParams struct {
	Code        string   `json:"code" jsonschema:"The TypeScript or JavaScript code to execute"`
	Permissions []string `json:"permissions,omitempty" jsonschema:"Optional Deno permissions (e.g., --allow-net, --allow-read)"`
}

// ExecuteDenoCodeResults defines the output of executing Deno code
type ExecuteDenoCodeResults struct {
	Status       string `json:"status"`
	Output       string `json:"output,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// DenoExecutorHandler handles Deno code execution
type DenoExecutorHandler struct {
	mu sync.Mutex
}

// ExecuteCode executes TypeScript/JavaScript code using Deno
func (h *DenoExecutorHandler) ExecuteCode(ctx tool.Context, params ExecuteDenoCodeParams) ExecuteDenoCodeResults {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Build Deno command with permissions
	args := []string{"eval"}

	// Add permissions if specified
	if len(params.Permissions) > 0 {
		args = append(args, params.Permissions...)
	} else {
		// Default permissions for safety
		args = append(args, "--allow-net", "--allow-read=/tmp", "--allow-write=/tmp")
	}

	// Add the code to execute
	args = append(args, params.Code)

	// Create command
	cmd := exec.Command("deno", args...)

	// Capture output
	output, err := cmd.CombinedOutput()

	if err != nil {
		return ExecuteDenoCodeResults{
			Status:       "error",
			Output:       string(output),
			ErrorMessage: fmt.Sprintf("Execution failed: %v", err),
		}
	}

	return ExecuteDenoCodeResults{
		Status: "success",
		Output: string(output),
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

	// Create Deno executor handler
	denoHandler := &DenoExecutorHandler{}

	// Create Deno code execution tool using functiontool
	denoTool, err := functiontool.New(
		functiontool.Config{
			Name:        "execute_deno_code",
			Description: "Executes TypeScript or JavaScript code securely using Deno runtime. Use this tool to run code snippets, perform calculations, or test scripts. The code runs in an isolated subprocess with configurable permissions.",
		},
		denoHandler.ExecuteCode,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Deno tool: %w", err)
	}

	// Create ADK agent
	agent, err := llmagent.New(llmagent.Config{
		Name:  "irc_agent",
		Model: model,
		Description: "An intelligent IRC bot that listens to messages and responds to users in the IRC channel.",
		Instruction: fmt.Sprintf(`You are a helpful IRC bot in the %s channel.
Your role is to assist users with their questions and engage in friendly conversation.
When users ask you questions or mention you, provide helpful and concise responses.
Your responses are automatically sent to the IRC channel, so just respond naturally.
Keep your responses brief and appropriate for IRC chat (usually 1-2 lines).
You have access to tools that will be displayed to users when used.
You can execute TypeScript/JavaScript code using the execute_deno_code tool when users ask you to run code or perform calculations.`, channel),
		Tools: []tool.Tool{
			ircTool,
			denoTool,
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
