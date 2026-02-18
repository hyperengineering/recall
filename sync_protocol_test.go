package recall

import (
	"encoding/json"
	"testing"
)

// ============================================================================
// Story 10.1: Sync Protocol DTOs & API Path Routing
// ============================================================================

// --- AC #1: SyncPushRequest type ---

func TestSyncPushRequest_JSONRoundtrip(t *testing.T) {
	req := SyncPushRequest{
		PushID:        "550e8400-e29b-41d4-a716-446655440000",
		SourceID:      "test-source",
		SchemaVersion: 2,
		Entries: []ChangeLogEntry{
			{
				Sequence:  1,
				TableName: "lore_entries",
				EntityID:  "01ABC",
				Operation: "upsert",
				Payload:   json.RawMessage(`{"id":"01ABC","content":"hello"}`),
				CreatedAt: "2026-01-28T10:00:00Z",
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify snake_case JSON tags
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	for _, key := range []string{"push_id", "source_id", "schema_version", "entries"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}

	// Verify roundtrip
	var decoded SyncPushRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.PushID != req.PushID {
		t.Errorf("PushID = %q, want %q", decoded.PushID, req.PushID)
	}
	if decoded.SchemaVersion != 2 {
		t.Errorf("SchemaVersion = %d, want 2", decoded.SchemaVersion)
	}
	if len(decoded.Entries) != 1 {
		t.Fatalf("Entries len = %d, want 1", len(decoded.Entries))
	}
}

// --- AC #2: ChangeLogEntry type (wire format) ---

func TestChangeLogEntry_JSONTags(t *testing.T) {
	entry := ChangeLogEntry{
		Sequence:  42,
		TableName: "lore_entries",
		EntityID:  "01ENTITY",
		Operation: "upsert",
		Payload:   json.RawMessage(`{"id":"01ENTITY"}`),
		CreatedAt: "2026-01-28T10:00:00Z",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	for _, key := range []string{"sequence", "table_name", "entity_id", "operation", "payload", "created_at"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestChangeLogEntry_NullablePayload(t *testing.T) {
	// Delete entries have null payload
	entry := ChangeLogEntry{
		Sequence:  43,
		TableName: "lore_entries",
		EntityID:  "01DEL",
		Operation: "delete",
		Payload:   nil,
		CreatedAt: "2026-01-28T10:00:00Z",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Payload should be null in JSON
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	if string(raw["payload"]) != "null" {
		t.Errorf("payload = %s, want null", string(raw["payload"]))
	}

	// Roundtrip: json.RawMessage stores literal "null" bytes (not Go nil)
	var decoded ChangeLogEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if string(decoded.Payload) != "null" {
		t.Errorf("Payload = %s, want null", string(decoded.Payload))
	}
}

func TestChangeLogEntry_PayloadIsRawJSON(t *testing.T) {
	// When payload is raw JSON, it should be embedded directly (not escaped)
	entry := ChangeLogEntry{
		Sequence:  1,
		TableName: "lore_entries",
		EntityID:  "01ABC",
		Operation: "upsert",
		Payload:   json.RawMessage(`{"id":"01ABC","content":"hello"}`),
		CreatedAt: "2026-01-28T10:00:00Z",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// The payload should be embedded as raw JSON, not as a string
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)

	// Parse the payload value — it should be an object, not a string
	var payloadObj map[string]interface{}
	if err := json.Unmarshal(raw["payload"], &payloadObj); err != nil {
		t.Fatalf("payload should be embedded JSON object, got: %s", string(raw["payload"]))
	}
	if payloadObj["id"] != "01ABC" {
		t.Errorf("payload.id = %v, want 01ABC", payloadObj["id"])
	}
}

// --- AC #3: SyncPushResponse type ---

func TestSyncPushResponse_JSONRoundtrip(t *testing.T) {
	resp := SyncPushResponse{
		Accepted:       5,
		RemoteSequence: 42,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)

	if _, ok := raw["accepted"]; !ok {
		t.Error("missing JSON key 'accepted'")
	}
	if _, ok := raw["remote_sequence"]; !ok {
		t.Error("missing JSON key 'remote_sequence'")
	}

	var decoded SyncPushResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.Accepted != 5 {
		t.Errorf("Accepted = %d, want 5", decoded.Accepted)
	}
	if decoded.RemoteSequence != 42 {
		t.Errorf("RemoteSequence = %d, want 42", decoded.RemoteSequence)
	}
}

// --- AC #4: SyncDeltaResponse and DeltaEntry types ---

func TestSyncDeltaResponse_JSONRoundtrip(t *testing.T) {
	resp := SyncDeltaResponse{
		Entries: []DeltaEntry{
			{
				Sequence:   100,
				TableName:  "lore_entries",
				EntityID:   "01DELTA",
				Operation:  "upsert",
				Payload:    json.RawMessage(`{"id":"01DELTA"}`),
				SourceID:   "remote-source",
				CreatedAt:  "2026-01-28T10:00:00Z",
				ReceivedAt: "2026-01-28T10:01:00Z",
			},
		},
		LastSequence:   100,
		LatestSequence: 200,
		HasMore:        true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)

	for _, key := range []string{"entries", "last_sequence", "latest_sequence", "has_more"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}

	var decoded SyncDeltaResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.HasMore != true {
		t.Error("HasMore should be true")
	}
	if decoded.LastSequence != 100 {
		t.Errorf("LastSequence = %d, want 100", decoded.LastSequence)
	}
	if decoded.LatestSequence != 200 {
		t.Errorf("LatestSequence = %d, want 200", decoded.LatestSequence)
	}
	if len(decoded.Entries) != 1 {
		t.Fatalf("Entries len = %d, want 1", len(decoded.Entries))
	}
}

func TestDeltaEntry_JSONTags(t *testing.T) {
	entry := DeltaEntry{
		Sequence:   1,
		TableName:  "lore_entries",
		EntityID:   "01ENT",
		Operation:  "upsert",
		Payload:    json.RawMessage(`{}`),
		SourceID:   "src",
		CreatedAt:  "2026-01-28T10:00:00Z",
		ReceivedAt: "2026-01-28T10:01:00Z",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)

	for _, key := range []string{"sequence", "table_name", "entity_id", "operation", "payload", "source_id", "created_at", "received_at"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

// --- AC #5: SyncValidationError (422 response) ---

func TestSyncValidationError_JSONRoundtrip(t *testing.T) {
	valErr := SyncValidationError{
		Accepted: 0,
		Errors: []EntryError{
			{
				Sequence:  1,
				TableName: "lore_entries",
				EntityID:  "01BAD",
				Code:      "INVALID_PAYLOAD",
				Message:   "missing required field: content",
			},
		},
	}

	data, err := json.Marshal(valErr)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)

	if _, ok := raw["accepted"]; !ok {
		t.Error("missing JSON key 'accepted'")
	}
	if _, ok := raw["errors"]; !ok {
		t.Error("missing JSON key 'errors'")
	}

	var decoded SyncValidationError
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.Accepted != 0 {
		t.Errorf("Accepted = %d, want 0", decoded.Accepted)
	}
	if len(decoded.Errors) != 1 {
		t.Fatalf("Errors len = %d, want 1", len(decoded.Errors))
	}

	e := decoded.Errors[0]
	if e.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1", e.Sequence)
	}
	if e.Code != "INVALID_PAYLOAD" {
		t.Errorf("Code = %q, want %q", e.Code, "INVALID_PAYLOAD")
	}
}

func TestEntryError_JSONTags(t *testing.T) {
	entry := EntryError{
		Sequence:  10,
		TableName: "lore_entries",
		EntityID:  "01ERR",
		Code:      "DUPLICATE",
		Message:   "duplicate entry",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)

	for _, key := range []string{"sequence", "table_name", "entity_id", "code", "message"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

// --- AC #6: SchemaMismatchError (409 response) ---

func TestSchemaMismatchError_JSONRoundtrip(t *testing.T) {
	schemaErr := SchemaMismatchError{
		ClientVersion: 1,
		ServerVersion: 2,
		Detail:        "client schema version 1 is behind server version 2",
	}

	data, err := json.Marshal(schemaErr)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)

	for _, key := range []string{"client_version", "server_version", "detail"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}

	var decoded SchemaMismatchError
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.ClientVersion != 1 {
		t.Errorf("ClientVersion = %d, want 1", decoded.ClientVersion)
	}
	if decoded.ServerVersion != 2 {
		t.Errorf("ServerVersion = %d, want 2", decoded.ServerVersion)
	}
}

// --- AC #7: Path helpers return /sync/* paths ---

func TestPushPath(t *testing.T) {
	syncer := &Syncer{storeID: "my-project"}
	got := syncer.pushPath()
	want := "/api/v1/stores/my-project/sync/push"
	if got != want {
		t.Errorf("pushPath() = %q, want %q", got, want)
	}
}

func TestDeltaPath_SyncProtocol(t *testing.T) {
	syncer := &Syncer{storeID: "my-project"}
	got := syncer.deltaPath()
	want := "/api/v1/stores/my-project/sync/delta"
	if got != want {
		t.Errorf("deltaPath() = %q, want %q", got, want)
	}
}

func TestSnapshotPath_SyncProtocol(t *testing.T) {
	syncer := &Syncer{storeID: "my-project"}
	got := syncer.snapshotPath()
	want := "/api/v1/stores/my-project/sync/snapshot"
	if got != want {
		t.Errorf("snapshotPath() = %q, want %q", got, want)
	}
}

func TestPushPath_EncodesStoreID(t *testing.T) {
	syncer := &Syncer{storeID: "neuralmux/engram"}
	got := syncer.pushPath()
	want := "/api/v1/stores/neuralmux%2Fengram/sync/push"
	if got != want {
		t.Errorf("pushPath() = %q, want %q", got, want)
	}
}

// --- AC #8: Path helpers require storeID ---

func TestPushPath_PanicsWithoutStoreID(t *testing.T) {
	syncer := &Syncer{storeID: ""}
	defer func() {
		if r := recover(); r == nil {
			t.Error("pushPath() should panic when storeID is empty")
		}
	}()
	syncer.pushPath()
}

func TestDeltaPath_PanicsWithoutStoreID(t *testing.T) {
	syncer := &Syncer{storeID: ""}
	defer func() {
		if r := recover(); r == nil {
			t.Error("deltaPath() should panic when storeID is empty")
		}
	}()
	syncer.deltaPath()
}

func TestSnapshotPath_PanicsWithoutStoreID(t *testing.T) {
	syncer := &Syncer{storeID: ""}
	defer func() {
		if r := recover(); r == nil {
			t.Error("snapshotPath() should panic when storeID is empty")
		}
	}()
	syncer.snapshotPath()
}
