# IRC Agent with Google ADK

An intelligent IRC bot powered by Google's Agent Development Kit (ADK) and Gemini AI. This bot listens to IRC messages and responds intelligently using the Gemini language model.

## Features

- **AI-Powered Responses**: Uses Google Gemini to generate intelligent responses to IRC messages
- **Custom IRC Tool**: Built-in tool for sending messages to IRC channels
- **Dual Mode Operation**:
  - IRC mode: Runs as a traditional IRC bot
  - Web mode: Provides a web interface for testing and development
- **Event-Driven**: Responds to messages that mention the bot or start with command prefixes (!, ,)

## Prerequisites

- Go 1.24.4 or later
- Google API Key (get from [Google AI Studio](https://aistudio.google.com/app/apikey))
- IRC server access with NickServ authentication

## Setup

### 1. Clone and Install Dependencies

```bash
git clone <your-repo-url>
cd irc-agent
go mod tidy
```

### 2. Configure Environment Variables

Copy the example environment file and fill in your credentials:

```bash
cp .env.example .env
```

Edit `.env` with your configuration:

```bash
# IRC Server Configuration
SERVER=irc.example.com:6667
CHANNEL=#your-channel
PASS=your-nickserv-password

# Google Gemini API Key
GOOGLE_API_KEY=your-google-api-key-here
```

### 3. Load Environment Variables

```bash
source .env
```

## Running the Agent

### IRC Mode (Production)

Run the agent to connect to IRC and start responding to messages:

```bash
go run agent.go
```

The bot will:
1. Connect to the IRC server
2. Authenticate with NickServ
3. Join the configured channel
4. Listen for messages that mention "layer-d8" or start with `!` or `,`
5. Respond using the Gemini AI model

### Web Interface Mode (Development)

Run the agent with a web interface for testing:

```bash
go run agent.go web api webui
```

Access the web interface at [http://localhost:8080](http://localhost:8080) to chat with the agent.

## Architecture

### Components

1. **IRCMessageTool**: Custom ADK tool that sends messages to the IRC channel
   - Implements the `tool.Tool` interface
   - Thread-safe message sending with mutex locks
   - Returns structured response with status and metadata

2. **IRCAgent**: Wraps the ADK agent with IRC functionality
   - Manages IRC connection lifecycle
   - Routes IRC messages to the ADK agent
   - Processes agent responses and sends them to IRC

3. **ADK Integration**: Uses Google's Agent Development Kit
   - Gemini 2.0 Flash model for fast responses
   - Custom instructions for IRC-appropriate responses
   - Tool-based architecture for extensibility

### Message Flow

```
IRC Message → IRC Event Handler → ADK Agent → Tool Execution → IRC Response
```

1. User sends message in IRC channel
2. Bot detects mention or command prefix
3. Message is formatted and sent to ADK agent
4. Agent processes with Gemini model
5. Agent calls `send_irc_message` tool
6. Tool sends response back to IRC channel

## Customization

### Modify Agent Behavior

Edit the `Instruction` field in `agent.go:86` to change how the bot responds:

```go
Instruction: fmt.Sprintf(`You are a helpful IRC bot in the %s channel.
Your role is to assist users with their questions and engage in friendly conversation.
...`, channel),
```

### Add More Tools

Add additional tools to the agent's `Tools` array in `agent.go:90`:

```go
Tools: []tool.Tool{
    ircTool,
    // Add more tools here
},
```

### Change Trigger Conditions

Modify the bot mention detection in `agent.go:149`:

```go
botMentioned := strings.Contains(strings.ToLower(message), "layer-d8") ||
    strings.HasPrefix(message, "!") ||
    strings.HasPrefix(message, ",")
```

## Legacy Bot

The original simple date bot is still available in `main.go`. It responds to `,date` commands with the current date.

## Docker Deployment

The project includes Docker configuration for deployment:

```bash
docker-compose up -d
```

Make sure to create a `.env` file with your configuration before running.

## License

BSD 3-Clause License - See LICENSE file for details

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.
