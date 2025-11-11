package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/thoj/go-ircevent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher/adk"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/server/restapi/services"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

// IRCMessageTool is a custom tool for sending messages to an IRC channel
type IRCMessageTool struct {
	conn    *irc.Connection
	channel string
	mu      sync.Mutex
}

// Name returns the name of the tool
func (t *IRCMessageTool) Name() string {
	return "send_irc_message"
}

// Description returns the description of the tool
func (t *IRCMessageTool) Description() string {
	return "Sends a message to the IRC channel. Use this tool to respond to users in the IRC channel."
}

// InputSchema returns the JSON schema for the tool's input parameters
func (t *IRCMessageTool) InputSchema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "The message to send to the IRC channel",
			},
		},
		"required": []string{"message"},
	}
}

// Call executes the tool with the given input
func (t *IRCMessageTool) Call(ctx context.Context, input map[string]interface{}) (interface{}, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	message, ok := input["message"].(string)
	if !ok {
		return nil, fmt.Errorf("message must be a string")
	}

	if t.conn == nil {
		return nil, fmt.Errorf("IRC connection not initialized")
	}

	// Send the message to the channel
	t.conn.Privmsg(t.channel, message)

	return map[string]interface{}{
		"status":  "sent",
		"message": message,
		"channel": t.channel,
	}, nil
}

// IRCAgent wraps the ADK agent with IRC functionality
type IRCAgent struct {
	agent   *llmagent.Agent
	ircConn *irc.Connection
	channel string
	tool    *IRCMessageTool
}

// NewIRCAgent creates a new IRC agent with ADK integration
func NewIRCAgent(ctx context.Context) (*IRCAgent, error) {
	// Get environment variables
	server := os.Getenv("SERVER")
	channel := os.Getenv("CHANNEL")
	password := os.Getenv("PASS")
	apiKey := os.Getenv("GOOGLE_API_KEY")

	if server == "" || channel == "" || password == "" {
		return nil, fmt.Errorf("SERVER, CHANNEL, and PASS environment variables are required")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY environment variable is required")
	}

	// Create IRC connection
	ircConn := irc.IRC("layer-d8", "layer-d8")
	ircConn.UseTLS = false

	// Create Gemini model
	model, err := gemini.NewModel(ctx, "gemini-2.0-flash-exp", &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create model: %w", err)
	}

	// Create IRC message tool
	ircTool := &IRCMessageTool{
		conn:    ircConn,
		channel: channel,
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
Keep your responses brief and appropriate for IRC chat (usually 1-2 lines).`, channel),
		Tools: []tool.Tool{
			ircTool,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	return &IRCAgent{
		agent:   agent,
		ircConn: ircConn,
		channel: channel,
		tool:    ircTool,
	}, nil
}

// Start connects to IRC and starts listening for messages
func (ia *IRCAgent) Start(ctx context.Context) error {
	password := os.Getenv("PASS")
	server := os.Getenv("SERVER")

	// Set up IRC event handlers
	ia.ircConn.AddCallback("001", func(e *irc.Event) {
		log.Printf("Connected to IRC server")
		ia.ircConn.Privmsg("nickserv", fmt.Sprintf("identify %s", password))
		log.Printf("Authenticated with NickServ")

		// Wait a bit then join channel
		go func() {
			<-ctx.Done()
		}()

		ia.ircConn.Join(ia.channel)
		log.Printf("Joined channel: %s", ia.channel)
	})

	// Handle PRIVMSG events
	ia.ircConn.AddCallback("PRIVMSG", func(e *irc.Event) {
		message := e.Message()
		sender := e.Nick

		log.Printf("[%s] <%s> %s", ia.channel, sender, message)

		// Check if the bot is mentioned or if message starts with a command
		botMentioned := strings.Contains(strings.ToLower(message), "layer-d8") ||
			strings.HasPrefix(message, "!") ||
			strings.HasPrefix(message, ",")

		if botMentioned {
			// Process message with ADK agent
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

// processMessage sends the IRC message to the ADK agent for processing
func (ia *IRCAgent) processMessage(ctx context.Context, sender, message string) {
	// Create a prompt for the agent
	prompt := fmt.Sprintf("User %s said: %s\n\nPlease respond appropriately using the send_irc_message tool.", sender, message)

	// Send to agent (this would use the agent's chat/completion API)
	log.Printf("Processing message from %s: %s", sender, message)

	// For now, we'll log that we would process this
	// In a full implementation, you would call the agent's API here
	// For example: ia.agent.Chat(ctx, prompt)

	log.Printf("Agent would process: %s", prompt)
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
