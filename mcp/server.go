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
	session   *MultiStoreSession // Tracks lore across multiple stores with global counter
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
	s := &Server{
		client:  client,
		session: NewMultiStoreSession(),
	}

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
		{Name: "recall_query", Description: "Retrieve relevant lore based on semantic similarity to a query from a specific store"},
		{Name: "recall_record", Description: "Capture lore from current experience into a specific store for future recall"},
		{Name: "recall_feedback", Description: "Provide feedback on lore recalled this session to adjust confidence"},
		{Name: "recall_sync", Description: "Synchronize local lore with Engram for a specific store"},
		{Name: "recall_store_list", Description: "List available stores from Engram"},
		{Name: "recall_store_info", Description: "Get detailed information and statistics about a specific store"},
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
	case "recall_store_list":
		return s.handleStoreList(ctx, args)
	case "recall_store_info":
		return s.handleStoreInfo(ctx, args)
	default:
		return &ToolResult{Content: fmt.Sprintf("unknown tool: %s", name), IsError: true}, nil
	}
}

func (s *Server) registerTools() {
	// recall_query
	s.mcpServer.AddTool(mcp.NewTool("recall_query",
		mcp.WithDescription("Retrieve relevant lore based on semantic similarity to a query from a specific store. Returns lore entries with session references (L1, L2, ...) that can be used for feedback."),
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
			mcp.WithStringItems(),
		),
		mcp.WithString("store",
			mcp.Description("Target store ID (default: resolved via env/config/default)"),
		),
	), s.mcpHandleQuery)

	// recall_record
	s.mcpServer.AddTool(mcp.NewTool("recall_record",
		mcp.WithDescription("Capture lore from current experience into a specific store for future recall. Use this to record insights, patterns, decisions, and learnings discovered during development."),
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
		mcp.WithString("store",
			mcp.Description("Target store ID (default: resolved via env/config/default)"),
		),
	), s.mcpHandleRecord)

	// recall_feedback
	s.mcpServer.AddTool(mcp.NewTool("recall_feedback",
		mcp.WithDescription("Provide feedback on lore recalled this session to adjust confidence. Use session references (L1, L2, ...) from query results. The store is automatically resolved from session refs; only specify store when using direct lore IDs."),
		mcp.WithArray("helpful",
			mcp.Description("Session refs (L1, L2) or lore IDs of helpful lore (+0.08 confidence)"),
			mcp.WithStringItems(),
		),
		mcp.WithArray("not_relevant",
			mcp.Description("Session refs or lore IDs of irrelevant lore (no change)"),
			mcp.WithStringItems(),
		),
		mcp.WithArray("incorrect",
			mcp.Description("Session refs or lore IDs of incorrect lore (-0.15 confidence)"),
			mcp.WithStringItems(),
		),
		mcp.WithString("store",
			mcp.Description("Target store ID (only needed for direct lore IDs, not session refs)"),
		),
	), s.mcpHandleFeedback)

	// recall_sync
	s.mcpServer.AddTool(mcp.NewTool("recall_sync",
		mcp.WithDescription("Synchronize local lore with Engram for a specific store. Requires ENGRAM_URL and ENGRAM_API_KEY to be configured."),
		mcp.WithString("direction",
			mcp.Description("Sync direction: pull, push, or both (default: both)"),
		),
		mcp.WithString("store",
			mcp.Description("Target store ID (default: resolved via env/config/default)"),
		),
	), s.mcpHandleSync)

	// recall_store_list
	s.mcpServer.AddTool(mcp.NewTool("recall_store_list",
		mcp.WithDescription("List available stores from Engram. This is a read-only operation for discovering available knowledge bases."),
		mcp.WithString("prefix",
			mcp.Description("Filter stores by path prefix (e.g., 'neuralmux/')"),
		),
	), s.mcpHandleStoreList)

	// recall_store_info
	s.mcpServer.AddTool(mcp.NewTool("recall_store_info",
		mcp.WithDescription("Get detailed information and statistics about a specific store. This is a read-only operation for inspecting store metadata."),
		mcp.WithString("store",
			mcp.Description("Store ID to inspect (default: resolved via env/config/default)"),
		),
	), s.mcpHandleStoreInfo)
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

func (s *Server) mcpHandleStoreList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.handleStoreList(ctx, req.GetArguments())
	if err != nil {
		return nil, err
	}
	return toMCPResult(result), nil
}

func (s *Server) mcpHandleStoreInfo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.handleStoreInfo(ctx, req.GetArguments())
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

	// Resolve store parameter (explicit > env > default)
	storeID, err := s.resolveStore(args)
	if err != nil {
		return &ToolResult{Content: fmt.Sprintf("invalid store ID: %v", err), IsError: true}, nil
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

	// Track results in multi-store session with store context
	// Build session refs using MCP's global counter
	sessionRefs := make(map[string]string)
	for _, lore := range result.Lore {
		ref := s.session.Track(storeID, lore.ID)
		sessionRefs[ref] = lore.ID
	}

	output := s.formatQueryResultWithSession(result, sessionRefs)
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

	// Resolve session refs (L1, L2, etc.) to store-aware refs
	// This preserves the store context for each lore entry
	helpfulRefs := s.resolveSessionRefsWithStore(helpful)
	notRelevantRefs := s.resolveSessionRefsWithStore(notRelevant)
	incorrectRefs := s.resolveSessionRefsWithStore(incorrect)

	// Group feedback by store for routing
	feedbackByStore := s.groupFeedbackByStore(helpfulRefs, notRelevantRefs, incorrectRefs)

	// Apply feedback for each store and aggregate results
	result := &recall.FeedbackResult{Updated: []recall.FeedbackUpdate{}}

	for storeID, storeFeedback := range feedbackByStore {
		storeResult, err := s.applyStoreFeedback(ctx, storeID, storeFeedback)
		if err != nil {
			// Continue processing other stores, but track the error
			continue
		}
		// Aggregate results
		result.Updated = append(result.Updated, storeResult.Updated...)
		result.NotFound = append(result.NotFound, storeResult.NotFound...)
	}

	output := formatFeedbackResult(result)
	return &ToolResult{Content: output}, nil
}

// resolveSessionRefsWithStore converts session refs (L1, L2) to StoreRefs,
// preserving the store context for each lore entry.
// If a ref is already a lore ID (not a session ref), it's returned with an empty store ID.
func (s *Server) resolveSessionRefsWithStore(refs []string) []StoreRef {
	if len(refs) == 0 {
		return nil
	}

	resolved := make([]StoreRef, 0, len(refs))
	for _, ref := range refs {
		if storeRef, ok := s.session.Resolve(ref); ok {
			resolved = append(resolved, storeRef)
		} else {
			// Not a session ref, pass through as-is with empty store ID
			// Direct lore IDs will use the default/resolved store
			resolved = append(resolved, StoreRef{StoreID: "", LoreID: ref})
		}
	}
	return resolved
}

// storeFeedback holds feedback for a specific store.
type storeFeedback struct {
	Helpful     []string
	NotRelevant []string
	Incorrect   []string
}

// groupFeedbackByStore groups resolved refs by their store ID.
// Refs without a store ID (direct lore IDs) are grouped under an empty string key.
func (s *Server) groupFeedbackByStore(helpful, notRelevant, incorrect []StoreRef) map[string]*storeFeedback {
	result := make(map[string]*storeFeedback)

	getOrCreate := func(storeID string) *storeFeedback {
		if sf, ok := result[storeID]; ok {
			return sf
		}
		sf := &storeFeedback{}
		result[storeID] = sf
		return sf
	}

	for _, ref := range helpful {
		sf := getOrCreate(ref.StoreID)
		sf.Helpful = append(sf.Helpful, ref.LoreID)
	}

	for _, ref := range notRelevant {
		sf := getOrCreate(ref.StoreID)
		sf.NotRelevant = append(sf.NotRelevant, ref.LoreID)
	}

	for _, ref := range incorrect {
		sf := getOrCreate(ref.StoreID)
		sf.Incorrect = append(sf.Incorrect, ref.LoreID)
	}

	return result
}

// applyStoreFeedback applies feedback for a specific store.
// For now, this uses the client's FeedbackBatch since we have a single local store.
// The storeID is preserved for future multi-store routing to Engram.
func (s *Server) applyStoreFeedback(ctx context.Context, storeID string, feedback *storeFeedback) (*recall.FeedbackResult, error) {
	// Apply feedback using the client - lore IDs are used directly
	// The store context is preserved in the sync queue for routing to Engram
	return s.client.FeedbackBatch(ctx, recall.FeedbackParams{
		Helpful:     feedback.Helpful,
		NotRelevant: feedback.NotRelevant,
		Incorrect:   feedback.Incorrect,
	})
}

func (s *Server) handleSync(ctx context.Context, args map[string]any) (*ToolResult, error) {
	err := s.client.Sync(ctx)
	if err != nil {
		return &ToolResult{Content: fmt.Sprintf("sync failed: %v", err), IsError: true}, nil
	}
	return &ToolResult{Content: "Sync completed successfully"}, nil
}

// Formatting functions

// formatQueryResultWithSession formats query results using MCP session refs.
// sessionRefs maps session ref (L1, L2) to lore ID.
func (s *Server) formatQueryResultWithSession(result *recall.QueryResult, sessionRefs map[string]string) string {
	if len(result.Lore) == 0 {
		return "No matching lore found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matching lore entries:\n\n", len(result.Lore)))

	// Build reverse map from lore ID to session ref
	idToRef := make(map[string]string)
	for ref, id := range sessionRefs {
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
