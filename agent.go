package main

import (
	"context"
	"fmt"
	"log"
	"os"
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
	conn    *irc.Connection
	channel string
	mu      sync.Mutex
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

	// Send the message to the channel
	h.conn.Privmsg(h.channel, params.Message)

	return SendIRCMessageResults{
		Status:  "success",
		Message: params.Message,
		Channel: h.channel,
	}
}

const (
	// MessageWindowSize is the number of messages to keep in the sliding window
	MessageWindowSize = 20
	// SummarizationThreshold is when to trigger summarization (80% of window)
	SummarizationThreshold = 16
)

// IRCAgent wraps the ADK agent with IRC functionality
type IRCAgent struct {
	agent          agent.Agent
	runner         *runner.Runner
	sessionService session.Service
	memoryService  memory.Service
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
		conn:    ircConn,
		channel: channel,
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

	// Create ADK agent
	agent, err := llmagent.New(llmagent.Config{
		Name:  "irc_agent",
		Model: model,
		Description: "An intelligent IRC bot that listens to messages and responds to users in the IRC channel.",
		Instruction: fmt.Sprintf(`You are a helpful IRC bot in the %s channel.
Your role is to assist users with their questions and engage in friendly conversation.
When users ask you questions or mention you, provide helpful and concise responses.
Always use the send_irc_message tool to respond to users.
Keep your responses brief and appropriate for IRC chat (usually 1-2 lines).

IMPORTANT: You have access to a memory system that stores summaries of past conversations.
If you need context from earlier in the conversation that isn't in the recent message history,
check your memories - they contain important information from past interactions with this user.`, channel),
		Tools: []tool.Tool{
			ircTool,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// Create session service
	sessionService := session.InMemoryService()

	// Create memory service
	memoryService := memory.InMemoryService()

	// Create runner with in-memory services
	agentRunner, err := runner.New(runner.Config{
		AppName:         "irc_agent",
		Agent:           agent,
		SessionService:  sessionService,
		ArtifactService: artifact.InMemoryService(),
		MemoryService:   memoryService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create runner: %w", err)
	}

	return &IRCAgent{
		agent:          agent,
		runner:         agentRunner,
		sessionService: sessionService,
		memoryService:  memoryService,
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
		ia.ircConn.Join(ia.channel)
		log.Printf("Joined channel: %s", ia.channel)
	})

	// Handle PRIVMSG events
	ia.ircConn.AddCallback("PRIVMSG", func(e *irc.Event) {
		message := e.Message()
		sender := e.Nick

		log.Printf("[%s] <%s> %s", ia.channel, sender, message)

		if e.Nick != "agent" {
			go ia.processMessage(ctx, sender, message)
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

// getMessageCount retrieves the message count from session state
func (ia *IRCAgent) getMessageCount(ctx context.Context, sessionID, userID string) int {
	resp, err := ia.sessionService.Get(ctx, &session.GetRequest{
		AppName:   "irc_agent",
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return 0
	}

	countVal, err := resp.Session.State().Get("message_count")
	if err != nil {
		return 0
	}

	if count, ok := countVal.(float64); ok {
		return int(count)
	}
	if count, ok := countVal.(int); ok {
		return count
	}
	return 0
}

// incrementMessageCount increments the message count in session state
func (ia *IRCAgent) incrementMessageCount(ctx context.Context, sessionID, userID string) error {
	resp, err := ia.sessionService.Get(ctx, &session.GetRequest{
		AppName:   "irc_agent",
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		return err
	}

	count := ia.getMessageCount(ctx, sessionID, userID)
	return resp.Session.State().Set("message_count", count+1)
}

// summarizeAndStore adds the current session to memory service before resetting
func (ia *IRCAgent) summarizeAndStore(ctx context.Context, sessionID, userID string) error {
	log.Printf("Storing conversation history to memory for user %s (threshold reached)", userID)

	// Get the current session
	resp, err := ia.sessionService.Get(ctx, &session.GetRequest{
		AppName:   "irc_agent",
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		log.Printf("Error getting session: %v", err)
		return err
	}

	// Add the session to memory service - the memory service will extract important information
	err = ia.memoryService.AddSession(ctx, resp.Session)
	if err != nil {
		log.Printf("Error adding session to memory: %v", err)
		return err
	}

	log.Printf("Successfully stored session to memory for user %s", userID)
	return nil
}

// resetSessionHistory resets the session to implement the sliding window
func (ia *IRCAgent) resetSessionHistory(ctx context.Context, sessionID, userID string) error {
	log.Printf("Resetting session history for user %s (sliding window)", userID)

	// Delete the old session
	err := ia.sessionService.Delete(ctx, &session.DeleteRequest{
		AppName:   "irc_agent",
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		log.Printf("Error deleting old session: %v", err)
	}

	// Create a new session
	_, err = ia.sessionService.Create(ctx, &session.CreateRequest{
		AppName:   "irc_agent",
		UserID:    userID,
		SessionID: sessionID,
		State:     map[string]any{"message_count": 0},
	})
	if err != nil {
		log.Printf("Error creating new session: %v", err)
		return err
	}

	log.Printf("Successfully reset session for user %s", userID)
	return nil
}

// processMessage sends the IRC message to the ADK agent for processing
func (ia *IRCAgent) processMessage(ctx context.Context, sender, message string) {
	// Create a prompt for the agent
	prompt := fmt.Sprintf("User %s said: %s\n\nPlease respond appropriately using the send_irc_message tool.", sender, message)

	log.Printf("Processing message from %s: %s", sender, message)

	// Create the content for the agent
	content := genai.NewContentFromText(prompt, genai.RoleUser)

	// Use a unique session ID for each user to maintain conversation history
	sessionID := fmt.Sprintf("irc-session-%s", sender)

	// Ensure session exists - create it if it doesn't
	_, err := ia.sessionService.Get(ctx, &session.GetRequest{
		AppName:   "irc_agent",
		UserID:    sender,
		SessionID: sessionID,
	})
	if err != nil {
		// Session doesn't exist, create it
		log.Printf("Creating new session for user %s", sender)
		_, err = ia.sessionService.Create(ctx, &session.CreateRequest{
			AppName:   "irc_agent",
			UserID:    sender,
			SessionID: sessionID,
			State:     map[string]any{"message_count": 0},
		})
		if err != nil {
			log.Printf("Error creating session: %v", err)
			return
		}
	}

	// Check if we need to summarize and reset (sliding window)
	messageCount := ia.getMessageCount(ctx, sessionID, sender)
	log.Printf("User %s message count: %d/%d", sender, messageCount, SummarizationThreshold)

	if messageCount >= SummarizationThreshold {
		// Summarize the conversation and store in memory
		if err := ia.summarizeAndStore(ctx, sessionID, sender); err != nil {
			log.Printf("Warning: Failed to summarize conversation: %v", err)
			// Continue anyway - we'll still reset to prevent unbounded growth
		}

		// Reset the session to clear old history (sliding window)
		if err := ia.resetSessionHistory(ctx, sessionID, sender); err != nil {
			log.Printf("Error resetting session: %v", err)
			return
		}
	}

	// Run the agent with the message
	runConfig := agent.RunConfig{}
	events := ia.runner.Run(ctx, sender, sessionID, content, runConfig)

	// Process the events
	for event, err := range events {
		if err != nil {
			log.Printf("Error processing message: %v", err)
			return
		}

		// Log events
		if event != nil {
			log.Printf("Agent event - Author: %s, InvocationID: %s", event.Author, event.InvocationID)

			// Log if this is a final response
			if event.IsFinalResponse() {
				log.Printf("Agent sent final response")
			}

			// Check if the content has function calls
			if event.Content != nil && len(event.Content.Parts) > 0 {
				for _, part := range event.Content.Parts {
					if part.FunctionCall != nil {
						log.Printf("Agent calling tool: %s", part.FunctionCall.Name)
					}
					if part.FunctionResponse != nil {
						log.Printf("Tool %s responded", part.FunctionResponse.Name)
					}
				}
			}
		}
	}

	// Increment message count after successful processing
	if err := ia.incrementMessageCount(ctx, sessionID, sender); err != nil {
		log.Printf("Warning: Failed to increment message count: %v", err)
	}

	log.Printf("Agent finished processing message from %s", sender)
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
