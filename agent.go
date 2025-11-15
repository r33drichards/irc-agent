package main

import (
	"context"
	"log"
	"os"

	"google.golang.org/adk/cmd/launcher/adk"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/server/restapi/services"
)

func main() {
	ctx := context.Background()

	// Get URL shortener host from environment (defaults to Railway production URL)
	shortenerHost := os.Getenv("SHORTENER_HOST")
	if shortenerHost == "" {
		shortenerHost = "https://irc-agent-production-09eb.up.railway.app"
	}

	// Create URL Shortener first
	urlShortener := NewURLShortener(shortenerHost)

	// Create IRC Agent with URL Shortener
	ircAgent, err := NewIRCAgent(ctx, urlShortener)
	if err != nil {
		log.Fatalf("Failed to create IRC agent: %v", err)
	}

	// Start URL Shortener on port 3000
	go func() {
		log.Println("Starting URL Shortener on port 3000...")
		if err := urlShortener.Serve("3000"); err != nil {
			log.Fatalf("URL Shortener failed: %v", err)
		}
	}()

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
