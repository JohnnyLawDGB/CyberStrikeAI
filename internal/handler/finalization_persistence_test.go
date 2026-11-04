package handler

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberstrike-ai/internal/agentfinalizer"
	"cyberstrike-ai/internal/database"
	"cyberstrike-ai/internal/mcp"

	"go.uber.org/zap"
)

func TestPersistFinalizationDecisionKeepsFailedExecutionIDs(t *testing.T) {
	logger := zap.NewNop()
	db, err := database.NewDB(filepath.Join(t.TempDir(), "finalization-persistence.db"), logger)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	conv, err := db.CreateConversation("test", database.ConversationCreateMeta{})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	msg, err := db.AddMessage(conv.ID, "assistant", "处理中...", nil)
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	if err := db.SaveToolExecution(&mcp.ToolExecution{
		ID:             "failed-exec",
		ToolName:       "test::tool",
		Arguments:      map[string]interface{}{"target": "concrete"},
		Status:         mcp.ToolExecutionStatusFailed,
		StartTime:      time.Now(),
		ConversationID: conv.ID,
	}); err != nil {
		t.Fatalf("SaveToolExecution: %v", err)
	}

	decision := agentfinalizer.Decide(db, agentfinalizer.Input{
		Response:                 "The tool failed.",
		ConversationID:           conv.ID,
		AssistantMessageID:       msg.ID,
		RequireExecutionEvidence: true,
	})
	h := &AgentHandler{db: db, logger: logger}
	h.persistFinalizationDecision(conv.ID, msg.ID, "test", nil, "reasoning", decision)

	messages, err := db.GetMessages(conv.ID)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	got := messages[0]
	if len(got.MCPExecutionIDs) != 1 || got.MCPExecutionIDs[0] != "failed-exec" {
		t.Fatalf("persisted execution ids = %v", got.MCPExecutionIDs)
	}
	if !strings.Contains(got.Content, agentfinalizer.ReasonExecutionFailed) {
		t.Fatalf("blocked message does not identify execution failure: %q", got.Content)
	}
}
