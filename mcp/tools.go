// Package mcp provides optional MCP (Model Context Protocol) tool adapters for Recall.
// This package allows Recall to be integrated with MCP-compatible agent frameworks.
//
// This package offers two approaches:
//
// 1. Full MCP Server (server.go) - RECOMMENDED
//    Use NewServer() for a complete MCP server implementation using mcp-go.
//    This provides full MCP protocol support with stdio transport.
//
// 2. Registry Pattern (tools.go) - LEGACY/ALTERNATIVE
//    Use RegisterTools() for framework-agnostic integration where you
//    provide your own MCP registry implementation. This is useful for
//    custom agent frameworks that already have MCP infrastructure.
//
// For most use cases, prefer the full MCP server approach.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hyperengineering/recall"
)

// Registry is an interface for MCP tool registration.
// Implement this interface to integrate Recall with your MCP framework.
type Registry interface {
	Register(tool Tool)
}

// Tool represents an MCP tool definition.
type Tool struct {
	Name        string
	Description string
	Parameters  Schema
	Handler     Handler
}

// Schema defines the JSON schema for tool parameters.
type Schema map[string]ParameterDef

// ParameterDef defines a single parameter.
type ParameterDef struct {
	Type        string            `json:"type"`
	Description string            `json:"description,omitempty"`
	Required    bool              `json:"required,omitempty"`
	Default     interface{}       `json:"default,omitempty"`
	Items       map[string]string `json:"items,omitempty"`
	Enum        []string          `json:"enum,omitempty"`
}

// Handler is a function that handles tool invocations.
type Handler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// RegisterTools registers Recall tools with an MCP registry.
// This is optional — Recall can be used without MCP.
//
// NOTE: For most use cases, prefer NewServer() which provides a complete
// MCP server implementation. Use RegisterTools only if you need to integrate
// with a custom MCP registry/framework that doesn't use the standard mcp-go
// server approach.
//
// The tools registered here mirror those in server.go but use your own
// registry interface for maximum flexibility.
func RegisterTools(registry Registry, client *recall.Client) {
	registry.Register(Tool{
		Name:        "recall_query",
		Description: "Retrieve relevant lore based on semantic similarity to a query",
		Parameters: Schema{
			"query": {
				Type:        "string",
				Description: "The search query to find relevant lore",
				Required:    true,
			},
			"k": {
				Type:        "integer",
				Description: "Maximum number of results to return",
				Default:     5,
			},
			"min_confidence": {
				Type:        "number",
				Description: "Minimum confidence threshold (0.0-1.0)",
				Default:     0.5,
			},
			"categories": {
				Type:        "array",
				Description: "Filter by specific categories",
				Items:       map[string]string{"type": "string"},
			},
		},
		Handler: makeQueryHandler(client),
	})

	registry.Register(Tool{
		Name:        "recall_record",
		Description: "Capture lore from current experience for future recall",
		Parameters: Schema{
			"content": {
				Type:        "string",
				Description: "The lore content - what was learned",
				Required:    true,
			},
			"category": {
				Type:        "string",
				Description: "Category of lore",
				Required:    true,
				Enum: []string{
					"ARCHITECTURAL_DECISION",
					"PATTERN_OUTCOME",
					"INTERFACE_LESSON",
					"EDGE_CASE_DISCOVERY",
					"IMPLEMENTATION_FRICTION",
					"TESTING_STRATEGY",
					"DEPENDENCY_BEHAVIOR",
					"PERFORMANCE_INSIGHT",
				},
			},
			"context": {
				Type:        "string",
				Description: "Additional context (story, epic, situation)",
			},
			"confidence": {
				Type:        "number",
				Description: "Initial confidence (0.0-1.0)",
				Default:     0.5,
			},
		},
		Handler: makeRecordHandler(client),
	})

	registry.Register(Tool{
		Name:        "recall_feedback",
		Description: "Provide feedback on lore recalled this session to adjust confidence",
		Parameters: Schema{
			"helpful": {
				Type:        "array",
				Description: "Session refs (L1, L2) or content snippets of helpful lore",
				Items:       map[string]string{"type": "string"},
			},
			"not_relevant": {
				Type:        "array",
				Description: "Session refs of lore that wasn't relevant to this context",
				Items:       map[string]string{"type": "string"},
			},
			"incorrect": {
				Type:        "array",
				Description: "Session refs of lore that was wrong or misleading",
				Items:       map[string]string{"type": "string"},
			},
		},
		Handler: makeFeedbackHandler(client),
	})
}

// queryParams represents the parameters for recall_query.
type queryParams struct {
	Query         string   `json:"query"`
	K             int      `json:"k"`
	MinConfidence float64  `json:"min_confidence"`
	Categories    []string `json:"categories"`
}

func makeQueryHandler(client *recall.Client) Handler {
	return func(ctx context.Context, rawParams json.RawMessage) (interface{}, error) {
		var params queryParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, fmt.Errorf("parse params: %w", err)
		}

		if params.Query == "" {
			return nil, fmt.Errorf("query is required")
		}

		qp := recall.QueryParams{
			Query: params.Query,
			K:     params.K,
		}
		if params.MinConfidence > 0 {
			qp.MinConfidence = &params.MinConfidence
		}

		for _, c := range params.Categories {
			qp.Categories = append(qp.Categories, recall.Category(c))
		}

		return client.Query(ctx, qp)
	}
}

// recordParams represents the parameters for recall_record.
type recordParams struct {
	Content    string  `json:"content"`
	Category   string  `json:"category"`
	Context    string  `json:"context"`
	Confidence float64 `json:"confidence"`
}

func makeRecordHandler(client *recall.Client) Handler {
	return func(_ context.Context, rawParams json.RawMessage) (interface{}, error) {
		var params recordParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, fmt.Errorf("parse params: %w", err)
		}

		if params.Content == "" {
			return nil, fmt.Errorf("content is required")
		}
		if params.Category == "" {
			return nil, fmt.Errorf("category is required")
		}

		// Build options
		opts := []recall.RecordOption{}
		if params.Context != "" {
			opts = append(opts, recall.WithContext(params.Context))
		}
		if params.Confidence != recall.ConfidenceDefault {
			opts = append(opts, recall.WithConfidence(params.Confidence))
		}

		return client.Record(params.Content, recall.Category(params.Category), opts...)
	}
}

// feedbackParams represents the parameters for recall_feedback.
type feedbackParams struct {
	Helpful     []string `json:"helpful"`
	NotRelevant []string `json:"not_relevant"`
	Incorrect   []string `json:"incorrect"`
}

func makeFeedbackHandler(client *recall.Client) Handler {
	return func(ctx context.Context, rawParams json.RawMessage) (interface{}, error) {
		var params feedbackParams
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return nil, fmt.Errorf("parse params: %w", err)
		}

		return client.FeedbackBatch(ctx, recall.FeedbackParams{
			Helpful:     params.Helpful,
			NotRelevant: params.NotRelevant,
			Incorrect:   params.Incorrect,
		})
	}
}
