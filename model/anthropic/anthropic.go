package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

type anthropicModel struct {
	client anthropic.Client
	name   anthropic.Model
}

// NewModel creates a new Anthropic model that implements the model.LLM interface.
// modelName should be something like "claude-3-5-haiku-20241022" for Haiku 3.5
func NewModel(ctx context.Context, modelName string, apiKey string) (model.LLM, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is required")
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	return &anthropicModel{
		name:   anthropic.Model(modelName),
		client: client,
	}, nil
}

func (m *anthropicModel) Name() string {
	return string(m.name)
}

// GenerateContent implements the model.LLM interface for Anthropic
func (m *anthropicModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	// Convert genai.Content to Anthropic messages
	messages, systemPrompt := convertToAnthropicMessages(req.Contents)

	// Build the Anthropic request
	params := anthropic.MessageNewParams{
		Model:     m.name,
		Messages:  messages,
		MaxTokens: 4096,
	}

	// Add system instruction from Config if present (ADK puts it here)
	if req.Config != nil && req.Config.SystemInstruction != nil {
		for _, part := range req.Config.SystemInstruction.Parts {
			if part.Text != "" {
				if systemPrompt != "" {
					systemPrompt += "\n\n"
				}
				systemPrompt += part.Text
			}
		}
	}

	// Add system prompt if present
	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemPrompt},
		}
	}

	// Add tools if present
	if req.Config != nil && len(req.Config.Tools) > 0 {
		tools := convertToAnthropicTools(req.Config.Tools)
		if len(tools) > 0 {
			params.Tools = tools
		}
	}

	// Set temperature if specified
	if req.Config != nil && req.Config.Temperature != nil {
		temp := float64(*req.Config.Temperature)
		params.Temperature = anthropic.Float(temp)
	}

	if stream {
		return m.generateStream(ctx, params)
	}

	return func(yield func(*model.LLMResponse, error) bool) {
		resp, err := m.generate(ctx, params)
		yield(resp, err)
	}
}

// generate calls the Anthropic API synchronously
func (m *anthropicModel) generate(ctx context.Context, params anthropic.MessageNewParams) (*model.LLMResponse, error) {
	resp, err := m.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to call Anthropic API: %w", err)
	}

	return convertToLLMResponse(resp), nil
}

// generateStream returns a stream of responses from Anthropic
func (m *anthropicModel) generateStream(ctx context.Context, params anthropic.MessageNewParams) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		stream := m.client.Messages.NewStreaming(ctx, params)

		var aggregatedText strings.Builder
		accumulated := &anthropic.Message{}

		for stream.Next() {
			event := stream.Current()

			// Accumulate the message
			if err := accumulated.Accumulate(event); err != nil {
				yield(nil, fmt.Errorf("accumulation error: %w", err))
				return
			}

			// Handle different event types for partial updates
			switch eventVariant := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				// Handle text deltas
				switch deltaVariant := eventVariant.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					aggregatedText.WriteString(deltaVariant.Text)

					// Yield partial response
					content := genai.NewContentFromText(aggregatedText.String(), genai.RoleModel)
					llmResp := &model.LLMResponse{
						Content:      content,
						Partial:      true,
						TurnComplete: false,
					}
					if !yield(llmResp, nil) {
						return
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			yield(nil, fmt.Errorf("stream error: %w", err))
			return
		}

		// Convert final accumulated message
		finalResp := convertToLLMResponse(accumulated)
		finalResp.TurnComplete = true
		yield(finalResp, nil)
	}
}

// convertToAnthropicMessages converts genai.Content to Anthropic messages
// Returns messages and system prompt separately
func convertToAnthropicMessages(contents []*genai.Content) ([]anthropic.MessageParam, string) {
	var messages []anthropic.MessageParam
	var systemPrompt string

	for _, content := range contents {
		if content == nil {
			continue
		}

		// Handle system messages separately
		if content.Role == "system" {
			for _, part := range content.Parts {
				if part.Text != "" {
					if systemPrompt != "" {
						systemPrompt += "\n\n"
					}
					systemPrompt += part.Text
				}
			}
			continue
		}

		// Build content blocks for this message
		var contentBlocks []anthropic.ContentBlockParamUnion
		for _, part := range content.Parts {
			if part.Text != "" {
				contentBlocks = append(contentBlocks, anthropic.NewTextBlock(part.Text))
			}

			// Handle function calls (tool uses)
			if part.FunctionCall != nil {
				toolUse := anthropic.NewToolUseBlock(
					part.FunctionCall.ID,
					part.FunctionCall.Args,
					part.FunctionCall.Name,
				)
				contentBlocks = append(contentBlocks, toolUse)
			}

			// Handle function responses (tool results)
			if part.FunctionResponse != nil {
				// Properly serialize the response as JSON
				var resultText string
				// Try to marshal to JSON for better serialization
				if jsonBytes, err := json.Marshal(part.FunctionResponse.Response); err == nil {
					resultText = string(jsonBytes)
				} else {
					// Fallback to string conversion if marshal fails
					resultText = fmt.Sprintf("%v", part.FunctionResponse.Response)
				}

				toolResult := anthropic.NewToolResultBlock(
					part.FunctionResponse.ID,
					resultText,
					false, // isError
				)
				contentBlocks = append(contentBlocks, toolResult)
			}
		}

		if len(contentBlocks) == 0 {
			continue
		}

		// Determine role
		var role anthropic.MessageParamRole
		if content.Role == genai.RoleModel || content.Role == "assistant" {
			role = anthropic.MessageParamRoleAssistant
		} else {
			// Default to user for any other role including genai.RoleUser
			role = anthropic.MessageParamRoleUser
		}

		messages = append(messages, anthropic.MessageParam{
			Role:    role,
			Content: contentBlocks,
		})
	}

	return messages, systemPrompt
}

// convertToAnthropicTools converts genai tools to Anthropic tools
func convertToAnthropicTools(genaiTools []*genai.Tool) []anthropic.ToolUnionParam {
	var tools []anthropic.ToolUnionParam

	for _, genaiTool := range genaiTools {
		if genaiTool == nil {
			continue
		}

		for _, fd := range genaiTool.FunctionDeclarations {
			if fd == nil {
				continue
			}

			// Create input schema from genai.Schema
			inputSchema := anthropic.ToolInputSchemaParam{
				Type:       "object",
				Properties: make(map[string]interface{}),
			}

			if fd.Parameters != nil {
				if fd.Parameters.Properties != nil {
					inputSchema.Properties = fd.Parameters.Properties
				}
				if fd.Parameters.Required != nil {
					inputSchema.Required = fd.Parameters.Required
				}
			}

			// Create tool with description using ToolParam directly
			toolParam := anthropic.ToolParam{
				Name:        fd.Name,
				InputSchema: inputSchema,
			}

			// Add description if present
			if fd.Description != "" {
				toolParam.Description = anthropic.String(fd.Description)
			}

			// Convert to ToolUnionParam using OfTool field
			tools = append(tools, anthropic.ToolUnionParam{
				OfTool: &toolParam,
			})
		}
	}

	return tools
}

// convertToLLMResponse converts an Anthropic message to an LLMResponse
func convertToLLMResponse(msg *anthropic.Message) *model.LLMResponse {
	if msg == nil {
		return &model.LLMResponse{}
	}

	// Build genai.Content from Anthropic response
	content := &genai.Content{
		Role:  genai.RoleModel,
		Parts: make([]*genai.Part, 0),
	}

	for _, block := range msg.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			content.Parts = append(content.Parts, &genai.Part{
				Text: b.Text,
			})

		case anthropic.ToolUseBlock:
			// Convert json.RawMessage to map[string]any
			var inputMap map[string]any
			if b.Input != nil {
				// Unmarshal the JSON input into a map
				if err := json.Unmarshal(b.Input, &inputMap); err != nil {
					// If unmarshal fails, create a simple map with the raw data
					inputMap = map[string]any{
						"_raw": string(b.Input),
					}
				}
			}

			content.Parts = append(content.Parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   b.ID,
					Name: b.Name,
					Args: inputMap,
				},
			})
		}
	}

	// Convert stop reason
	var finishReason genai.FinishReason
	switch msg.StopReason {
	case "end_turn":
		finishReason = genai.FinishReasonStop
	case "max_tokens":
		finishReason = genai.FinishReasonMaxTokens
	case "tool_use":
		finishReason = genai.FinishReasonStop
	default:
		finishReason = genai.FinishReasonOther
	}

	return &model.LLMResponse{
		Content: content,
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     int32(msg.Usage.InputTokens),
			CandidatesTokenCount: int32(msg.Usage.OutputTokens),
			TotalTokenCount:      int32(msg.Usage.InputTokens + msg.Usage.OutputTokens),
		},
		FinishReason: finishReason,
		TurnComplete: true,
	}
}
