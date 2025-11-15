# IRC Agent with Anthropic Claude

An intelligent IRC bot powered by Google's Agent Development Kit (ADK) and Anthropic's Claude AI. This bot listens to IRC messages and responds intelligently using the Claude 3.5 Haiku language model.

## Features

- **AI-Powered Responses**: Uses Anthropic Claude 3.5 Haiku to generate intelligent responses to IRC messages
- **Custom IRC Tool**: Built-in tool for sending messages to IRC channels
- **TypeScript/JavaScript Execution**: Execute TypeScript or JavaScript code using Deno with full permissions
- **Dual Mode Operation**:
  - IRC mode: Runs as a traditional IRC bot
  - Web mode: Provides a web interface for testing and development
- **Event-Driven**: Responds to messages that mention the bot or start with command prefixes (!, ,)

## Prerequisites

- Go 1.24.4 or later
- Anthropic API Key (get from [Anthropic Console](https://console.anthropic.com/))
- IRC server access with NickServ authentication
- Deno runtime (for TypeScript execution tool) - Install from [deno.land](https://deno.land)

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

# Anthropic API Key
ANTHROPIC_API_KEY=your-anthropic-api-key-here
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
5. Respond using the Claude 3.5 Haiku AI model

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

2. **TypeScriptExecutor**: Custom ADK tool for executing TypeScript/JavaScript code
   - Executes code in isolated temporary directories
   - Uses Deno runtime with `--allow-all` permissions
   - Returns stdout/stderr with proper error handling
   - Automatic cleanup of temporary files

3. **IRCAgent**: Wraps the ADK agent with IRC functionality
   - Manages IRC connection lifecycle
   - Routes IRC messages to the ADK agent
   - Processes agent responses and sends them to IRC

4. **ADK Integration**: Uses Google's Agent Development Kit
   - Claude 3.5 Haiku model for fast, intelligent responses
   - Custom instructions for IRC-appropriate responses
   - Tool-based architecture for extensibility

### Message Flow

```
IRC Message → IRC Event Handler → ADK Agent → Tool Execution → IRC Response
```

1. User sends message in IRC channel
2. Bot detects mention or command prefix
3. Message is formatted and sent to ADK agent
4. Agent processes with Claude 3.5 Haiku model
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

Add additional tools to the agent's `Tools` array. The agent currently includes:

1. **send_irc_message**: Sends messages to IRC channels
2. **execute_typescript**: Executes TypeScript/JavaScript code using Deno

To add more tools, follow the pattern in `agent.go`:

```go
Tools: []tool.Tool{
    ircTool,
    tsTool,
    // Add more tools here
},
```

### Using the TypeScript Execution Tool

The agent can execute TypeScript/JavaScript code using Deno. Users can ask the bot to:
- Perform calculations
- Run data transformations
- Test code snippets
- Execute algorithms
- Generate AWS S3 presigned URLs

Example IRC interactions:
```
<user> agent, calculate the sum of numbers from 1 to 100
<agent> [Using tool: execute_typescript]
<agent> The sum is 5050

<user> agent, write a function to check if a number is prime
<agent> [Using tool: execute_typescript]
<agent> Here's a prime checker function... [output]
```

To test the TypeScript executor directly:
```bash
go run -tags test_executor test_ts_executor.go agent.go
```

#### AWS S3 Presigned URLs Example

The agent can generate AWS S3 presigned URLs for both upload and download operations. Here's how to properly use AWS SDK v3 with the presigner:

**Important**: In AWS SDK v3, you must use the `getSignedUrl` function from `@aws-sdk/s3-request-presigner`, NOT `client.getSignedUrl()` (which doesn't exist).

```typescript
import { S3Client, GetObjectCommand, PutObjectCommand } from "npm:@aws-sdk/client-s3@3";
import { getSignedUrl } from "npm:@aws-sdk/s3-request-presigner@3";

const client = new S3Client({ region: "us-west-2" });

// Generate a presigned URL for PUT operation (upload)
const putCommand = new PutObjectCommand({
  Bucket: "your-bucket-name",
  Key: "path/to/file.jpg",
  ContentType: "image/jpeg",
});

const uploadUrl = await getSignedUrl(client, putCommand, { expiresIn: 86400 }); // 24 hours
console.log("Upload URL:", uploadUrl);

// Generate a presigned URL for GET operation (download)
const getCommand = new GetObjectCommand({
  Bucket: "your-bucket-name",
  Key: "path/to/file.jpg",
});

const downloadUrl = await getSignedUrl(client, getCommand, { expiresIn: 86400 }); // 24 hours
console.log("Download URL:", downloadUrl);
```

**Key Points:**
- Import `getSignedUrl` from `@aws-sdk/s3-request-presigner@3`
- Call `getSignedUrl(client, command, options)` - it's a standalone function, not a method on the client
- Use `PutObjectCommand` for upload URLs and `GetObjectCommand` for download URLs
- The `expiresIn` option sets the URL expiration time in seconds

**Complete Working Example:**

See [examples/aws_s3_presigned_urls.ts](examples/aws_s3_presigned_urls.ts) for a complete, runnable example with usage instructions.

These packages are pre-cached in the Docker container via `deps.ts` for faster execution.

**Testing Presigned URLs:**

You can test the presigned URL functionality directly using the included test script:

```bash
deno run --allow-net test_s3_presigned_url.ts
```

This will verify that the AWS SDK packages are properly installed and working correctly.

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
