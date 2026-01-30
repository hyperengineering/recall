package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hyperengineering/recall"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the MCP server with Recall tools.
type Server struct {
	client    *recall.Client
	mcpServer *server.MCPServer
}

// ToolResult represents the result of a tool call.
type ToolResult struct {
	Content string
	IsError bool
}

// ToolInfo represents a registered tool.
type ToolInfo struct {
	Name        string
	Description string
}

// NewServer creates a new MCP server with Recall tools registered.
func NewServer(client *recall.Client) *Server {
	s := &Server{client: client}

	// Create MCP server with metadata
	s.mcpServer = server.NewMCPServer(
		"recall",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register tools
	s.registerTools()

	return s
}

// Run starts the MCP server, reading from stdin and writing to stdout.
// It uses os.Stdin and os.Stdout internally via the mcp-go ServeStdio function.
func (s *Server) Run() error {
	return server.ServeStdio(s.mcpServer)
}

// HandleMessage processes a raw JSON-RPC message and returns a response.
// This is primarily for testing the MCP protocol layer.
func (s *Server) HandleMessage(ctx context.Context, message json.RawMessage) mcp.JSONRPCMessage {
	return s.mcpServer.HandleMessage(ctx, message)
}

// ListTools returns all registered tools.
func (s *Server) ListTools() []ToolInfo {
	return []ToolInfo{
		{Name: "recall_query", Description: "Retrieve relevant lore based on semantic similarity to a query"},
		{Name: "recall_record", Description: "Capture lore from current experience for future recall"},
		{Name: "recall_feedback", Description: "Provide feedback on lore recalled this session"},
		{Name: "recall_sync", Description: "Push pending local changes to Engram central service"},
	}
}

// CallTool executes a tool by name with the given arguments.
// This is used for testing and direct invocation.
func (s *Server) CallTool(ctx context.Context, name string, args map[string]any) (*ToolResult, error) {
	switch name {
	case "recall_query":
		return s.handleQuery(ctx, args)
	case "recall_record":
		return s.handleRecord(ctx, args)
	case "recall_feedback":
		return s.handleFeedback(ctx, args)
	case "recall_sync":
		return s.handleSync(ctx, args)
	default:
		return &ToolResult{Content: fmt.Sprintf("unknown tool: %s", name), IsError: true}, nil
	}
}

func (s *Server) registerTools() {
	// recall_query
	s.mcpServer.AddTool(mcp.NewTool("recall_query",
		mcp.WithDescription("Retrieve relevant lore based on semantic similarity to a query. Returns lore entries with session references (L1, L2, ...) that can be used for feedback."),
		mcp.WithString("query",
			mcp.Description("The search query to find relevant lore"),
			mcp.Required(),
		),
		mcp.WithNumber("k",
			mcp.Description("Maximum number of results to return (default: 5)"),
		),
		mcp.WithNumber("min_confidence",
			mcp.Description("Minimum confidence threshold 0.0-1.0 (default: 0.5)"),
		),
		mcp.WithArray("categories",
			mcp.Description("Filter by specific categories"),
		),
	), s.mcpHandleQuery)

	// recall_record
	s.mcpServer.AddTool(mcp.NewTool("recall_record",
		mcp.WithDescription("Capture lore from current experience for future recall. Use this to record insights, patterns, decisions, and learnings discovered during development."),
		mcp.WithString("content",
			mcp.Description("The lore content - what was learned (max 4000 chars)"),
			mcp.Required(),
		),
		mcp.WithString("category",
			mcp.Description("Category of lore: ARCHITECTURAL_DECISION, PATTERN_OUTCOME, INTERFACE_LESSON, EDGE_CASE_DISCOVERY, IMPLEMENTATION_FRICTION, TESTING_STRATEGY, DEPENDENCY_BEHAVIOR, PERFORMANCE_INSIGHT"),
			mcp.Required(),
		),
		mcp.WithString("context",
			mcp.Description("Additional context (story, epic, situation)"),
		),
		mcp.WithNumber("confidence",
			mcp.Description("Initial confidence 0.0-1.0 (default: 0.5)"),
		),
	), s.mcpHandleRecord)

	// recall_feedback
	s.mcpServer.AddTool(mcp.NewTool("recall_feedback",
		mcp.WithDescription("Provide feedback on lore recalled this session to adjust confidence. Use session references (L1, L2, ...) from query results."),
		mcp.WithArray("helpful",
			mcp.Description("Session refs (L1, L2) of helpful lore (+0.08 confidence)"),
		),
		mcp.WithArray("not_relevant",
			mcp.Description("Session refs of lore not relevant to this context (no change)"),
		),
		mcp.WithArray("incorrect",
			mcp.Description("Session refs of wrong or misleading lore (-0.15 confidence)"),
		),
	), s.mcpHandleFeedback)

	// recall_sync
	s.mcpServer.AddTool(mcp.NewTool("recall_sync",
		mcp.WithDescription("Push pending local changes to Engram central service. Requires ENGRAM_URL and ENGRAM_API_KEY to be configured."),
	), s.mcpHandleSync)
}

// MCP handlers that wrap internal handlers

func (s *Server) mcpHandleQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.handleQuery(ctx, req.GetArguments())
	if err != nil {
		return nil, err
	}
	return toMCPResult(result), nil
}

func (s *Server) mcpHandleRecord(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.handleRecord(ctx, req.GetArguments())
	if err != nil {
		return nil, err
	}
	return toMCPResult(result), nil
}

func (s *Server) mcpHandleFeedback(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.handleFeedback(ctx, req.GetArguments())
	if err != nil {
		return nil, err
	}
	return toMCPResult(result), nil
}

func (s *Server) mcpHandleSync(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.handleSync(ctx, req.GetArguments())
	if err != nil {
		return nil, err
	}
	return toMCPResult(result), nil
}

func toMCPResult(r *ToolResult) *mcp.CallToolResult {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: r.Content,
			},
		},
	}
	if r.IsError {
		result.IsError = true
	}
	return result
}

// Internal handlers

func (s *Server) handleQuery(ctx context.Context, args map[string]any) (*ToolResult, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return &ToolResult{Content: "query is required", IsError: true}, nil
	}

	qp := recall.QueryParams{
		Query: query,
	}

	if k, ok := args["k"].(float64); ok {
		qp.K = int(k)
	}

	if minConf, ok := args["min_confidence"].(float64); ok {
		qp.MinConfidence = &minConf
	}

	if cats, ok := args["categories"].([]any); ok {
		for _, c := range cats {
			if catStr, ok := c.(string); ok {
				qp.Categories = append(qp.Categories, recall.Category(catStr))
			}
		}
	}

	result, err := s.client.Query(ctx, qp)
	if err != nil {
		return &ToolResult{Content: fmt.Sprintf("query failed: %v", err), IsError: true}, nil
	}

	output := formatQueryResult(result)
	return &ToolResult{Content: output}, nil
}

func (s *Server) handleRecord(ctx context.Context, args map[string]any) (*ToolResult, error) {
	content, ok := args["content"].(string)
	if !ok || content == "" {
		return &ToolResult{Content: "content is required", IsError: true}, nil
	}

	categoryStr, ok := args["category"].(string)
	if !ok || categoryStr == "" {
		return &ToolResult{Content: "category is required", IsError: true}, nil
	}

	category := recall.Category(categoryStr)
	if !category.IsValid() {
		return &ToolResult{Content: fmt.Sprintf("invalid category: %s", categoryStr), IsError: true}, nil
	}

	opts := []recall.RecordOption{}
	if ctxStr, ok := args["context"].(string); ok && ctxStr != "" {
		opts = append(opts, recall.WithContext(ctxStr))
	}
	if conf, ok := args["confidence"].(float64); ok {
		opts = append(opts, recall.WithConfidence(conf))
	}

	lore, err := s.client.Record(content, category, opts...)
	if err != nil {
		return &ToolResult{Content: fmt.Sprintf("record failed: %v", err), IsError: true}, nil
	}

	output := formatRecordResult(lore)
	return &ToolResult{Content: output}, nil
}

func (s *Server) handleFeedback(ctx context.Context, args map[string]any) (*ToolResult, error) {
	var helpful, notRelevant, incorrect []string

	helpful = toStringSlice(args["helpful"])
	notRelevant = toStringSlice(args["not_relevant"])
	incorrect = toStringSlice(args["incorrect"])

	if len(helpful) == 0 && len(notRelevant) == 0 && len(incorrect) == 0 {
		return &ToolResult{Content: "at least one feedback type must be provided", IsError: true}, nil
	}

	result, err := s.client.FeedbackBatch(ctx, recall.FeedbackParams{
		Helpful:     helpful,
		NotRelevant: notRelevant,
		Incorrect:   incorrect,
	})
	if err != nil {
		return &ToolResult{Content: fmt.Sprintf("feedback failed: %v", err), IsError: true}, nil
	}

	output := formatFeedbackResult(result)
	return &ToolResult{Content: output}, nil
}

func (s *Server) handleSync(ctx context.Context, args map[string]any) (*ToolResult, error) {
	err := s.client.Sync(ctx)
	if err != nil {
		return &ToolResult{Content: fmt.Sprintf("sync failed: %v", err), IsError: true}, nil
	}
	return &ToolResult{Content: "Sync completed successfully"}, nil
}

// Formatting functions

func formatQueryResult(result *recall.QueryResult) string {
	if len(result.Lore) == 0 {
		return "No matching lore found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matching lore entries:\n\n", len(result.Lore)))

	// Build reverse map from ID to session ref
	idToRef := make(map[string]string)
	for ref, id := range result.SessionRefs {
		idToRef[id] = ref
	}

	for _, l := range result.Lore {
		ref := idToRef[l.ID]
		if ref == "" {
			ref = l.ID[:8] // fallback to truncated ID
		}
		sb.WriteString(fmt.Sprintf("[%s] %s\n", ref, l.Category))
		sb.WriteString(fmt.Sprintf("    %s\n", l.Content))
		sb.WriteString(fmt.Sprintf("    Confidence: %.2f\n\n", l.Confidence))
	}

	sb.WriteString("Use recall_feedback with session refs (L1, L2, ...) to rate helpfulness.")
	return sb.String()
}

func formatRecordResult(lore *recall.Lore) string {
	return fmt.Sprintf("Recorded lore [%s]:\n  Category: %s\n  Confidence: %.2f\n  Content: %s",
		lore.ID, lore.Category, lore.Confidence, truncate(lore.Content, 100))
}

func formatFeedbackResult(result *recall.FeedbackResult) string {
	var sb strings.Builder
	sb.WriteString("Feedback recorded:\n")

	if len(result.Updated) > 0 {
		sb.WriteString(fmt.Sprintf("  Updated: %d entries\n", len(result.Updated)))
		for _, u := range result.Updated {
			sb.WriteString(fmt.Sprintf("    - %s: confidence %.2f -> %.2f\n", u.ID[:8], u.Previous, u.Current))
		}
	}

	if len(result.NotFound) > 0 {
		sb.WriteString(fmt.Sprintf("  Not found: %d refs\n", len(result.NotFound)))
		for _, ref := range result.NotFound {
			sb.WriteString(fmt.Sprintf("    - %s\n", ref))
		}
	}

	return sb.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// toStringSlice converts various array types to []string.
// Handles []any, []string, and nil.
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}

	switch arr := v.(type) {
	case []string:
		return arr
	case []any:
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}
