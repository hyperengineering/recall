package recall_test

import (
	"encoding/json"
	"testing"

	"github.com/hyperengineering/recall"
)

func TestCategory_IsValid(t *testing.T) {
	validCategories := []recall.Category{
		recall.CategoryArchitecturalDecision,
		recall.CategoryPatternOutcome,
		recall.CategoryInterfaceLesson,
		recall.CategoryEdgeCaseDiscovery,
		recall.CategoryImplementationFriction,
		recall.CategoryTestingStrategy,
		recall.CategoryDependencyBehavior,
		recall.CategoryPerformanceInsight,
	}

	for _, cat := range validCategories {
		if !cat.IsValid() {
			t.Errorf("Category(%q).IsValid() = false, want true", cat)
		}
	}
}

func TestCategory_InvalidString(t *testing.T) {
	invalid := recall.Category("INVALID")
	if invalid.IsValid() {
		t.Error("Category(\"INVALID\").IsValid() = true, want false")
	}
}

func TestValidCategories_ReturnsAll8(t *testing.T) {
	cats := recall.ValidCategories()
	if len(cats) != 8 {
		t.Errorf("len(ValidCategories()) = %d, want 8", len(cats))
	}
}

func TestFeedbackType_ConstantsExist(t *testing.T) {
	tests := []struct {
		name string
		ft   recall.FeedbackType
		want string
	}{
		{"Helpful", recall.FeedbackHelpful, "helpful"},
		{"Incorrect", recall.FeedbackIncorrect, "incorrect"},
		{"NotRelevant", recall.FeedbackNotRelevant, "not_relevant"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.ft) != tt.want {
				t.Errorf("FeedbackType = %q, want %q", tt.ft, tt.want)
			}
		})
	}
}

func TestLore_JSONMarshal_SnakeCase(t *testing.T) {
	lore := recall.Lore{
		ID:              "test-id",
		Content:         "test content",
		Category:        recall.CategoryPatternOutcome,
		Confidence:      0.7,
		ValidationCount: 3,
		SourceID:        "src-1",
	}

	data, err := json.Marshal(lore)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	expectedKeys := []string{"id", "content", "category", "confidence", "validation_count", "source_id", "created_at", "updated_at"}
	for _, key := range expectedKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON key %q not found in marshaled output", key)
		}
	}

	// Embedding should be omitted (json:"-")
	if _, ok := m["embedding"]; ok {
		t.Error("Embedding should not appear in JSON output (tagged with json:\"-\")")
	}
}

func TestQueryParams_JSONMarshal_SnakeCase(t *testing.T) {
	minConf := 0.5
	params := recall.QueryParams{
		Query:         "test query",
		K:             5,
		MinConfidence: &minConf,
		Categories:    []recall.Category{recall.CategoryPatternOutcome},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	expectedKeys := []string{"query", "k", "min_confidence", "categories"}
	for _, key := range expectedKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON key %q not found in marshaled QueryParams", key)
		}
	}
}

func TestSyncQueueEntry_JSONMarshal_SnakeCase(t *testing.T) {
	entry := recall.SyncQueueEntry{
		ID:        1,
		LoreID:    "lore-123",
		Operation: "INSERT",
		Payload:   `{"outcome":"helpful"}`,
		Attempts:  2,
		LastError: "network timeout",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	expectedKeys := []string{"id", "lore_id", "operation", "payload", "queued_at", "attempts", "last_error"}
	for _, key := range expectedKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON key %q not found in marshaled SyncQueueEntry", key)
		}
	}
}

func TestFeedbackQueuePayload_JSONMarshal(t *testing.T) {
	payload := recall.FeedbackQueuePayload{
		Outcome: "helpful",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if _, ok := m["outcome"]; !ok {
		t.Error("JSON key \"outcome\" not found in marshaled FeedbackQueuePayload")
	}

	if m["outcome"] != "helpful" {
		t.Errorf("outcome = %q, want %q", m["outcome"], "helpful")
	}
}
