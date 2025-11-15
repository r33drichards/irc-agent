package main

import (
	"sync"

	irc "github.com/thoj/go-ircevent"
	"google.golang.org/adk/tool"
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
