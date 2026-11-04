package agentfinalizer

import (
	"path/filepath"
	"testing"
	"time"

	"cyberstrike-ai/internal/database"
	"cyberstrike-ai/internal/mcp"
	"cyberstrike-ai/internal/multiagent"

	"go.uber.org/zap"
)

func newDecisionTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.NewDB(filepath.Join(t.TempDir(), "finalizer.db"), zap.NewNop())
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func saveDecisionTestExecution(t *testing.T, db *database.DB, id, status string, conversationIDs ...string) {
	t.Helper()
	conversationID := ""
	if len(conversationIDs) > 0 {
		conversationID = conversationIDs[0]
	}
	if err := db.SaveToolExecution(&mcp.ToolExecution{
		ID:             id,
		ToolName:       "test::tool",
		Arguments:      map[string]interface{}{"input": id},
		Status:         status,
		StartTime:      time.Now(),
		ConversationID: conversationID,
	}); err != nil {
		t.Fatalf("SaveToolExecution(%s): %v", id, err)
	}
}

func TestDecideBlocksPendingToolExecutions(t *testing.T) {
	db := newDecisionTestDB(t)
	saveDecisionTestExecution(t, db, "run-queued", mcp.ToolExecutionStatusQueued)
	saveDecisionTestExecution(t, db, "run-running", mcp.ToolExecutionStatusRunning)
	saveDecisionTestExecution(t, db, "run-completed", mcp.ToolExecutionStatusCompleted)

	d := Decide(db, Input{
		Response:        "工具还没全部结束时，这只是一段候选输出。",
		MCPExecutionIDs: []string{"run-queued", "run-running", "run-completed"},
	})

	if d.Finalizable || d.Finalized {
		t.Fatalf("pending tools should not be finalizable: %+v", d)
	}
	if d.Status != StatusInProgress || d.CompletionReason != ReasonPendingTools {
		t.Fatalf("status/reason = %s/%s, want %s/%s", d.Status, d.CompletionReason, StatusInProgress, ReasonPendingTools)
	}
	if got, want := len(d.PendingExecutionIDs), 2; got != want {
		t.Fatalf("pending execution count = %d, want %d (%v)", got, want, d.PendingExecutionIDs)
	}
}

func TestDecideBlocksAwaitingHITLAndEmptyCandidate(t *testing.T) {
	hitl := Decide(nil, Input{Response: "等待人工审批", AwaitingHITL: true})
	if hitl.Finalizable || hitl.Status != StatusAwaitingHITL || hitl.CompletionReason != ReasonAwaitingHITL {
		t.Fatalf("HITL decision mismatch: %+v", hitl)
	}

	empty := Decide(nil, Input{Response: "⚠️ Eino 执行完成，但未捕获到助手文本输出。"})
	if empty.Finalizable || empty.Status != StatusBlocked || empty.CompletionReason != ReasonEmptyResponse {
		t.Fatalf("empty candidate decision mismatch: %+v", empty)
	}
}

func TestDecideBlocksWhenExecutionEvidenceIsRequiredButMissing(t *testing.T) {
	d := Decide(nil, Input{
		Response:                 "任务已处理完成。",
		RequireExecutionEvidence: true,
	})
	if d.Finalizable || d.Finalized {
		t.Fatalf("missing required execution evidence should not finalize: %+v", d)
	}
	if d.Status != StatusBlocked || d.CompletionReason != ReasonMissingEvidence {
		t.Fatalf("status/reason = %s/%s, want %s/%s", d.Status, d.CompletionReason, StatusBlocked, ReasonMissingEvidence)
	}
	if d.EvidenceVerified {
		t.Fatalf("missing required execution evidence should be marked unverified: %+v", d)
	}
	if len(d.MissingChecks) == 0 {
		t.Fatalf("missing checks should explain the evidence gap: %+v", d)
	}
}

func TestDecideBlocksWhenOnlyFailedEvidenceIsRecorded(t *testing.T) {
	db := newDecisionTestDB(t)
	saveDecisionTestExecution(t, db, "run-failed", mcp.ToolExecutionStatusFailed)
	saveDecisionTestExecution(t, db, "run-cancelled", mcp.ToolExecutionStatusCancelled)

	d := Decide(db, Input{
		Response:                 "任务已处理完成。",
		MCPExecutionIDs:          []string{"run-failed", "run-cancelled"},
		RequireExecutionEvidence: true,
	})

	if d.Finalizable || d.Finalized {
		t.Fatalf("failed evidence should not satisfy required execution evidence: %+v", d)
	}
	if d.Status != StatusFailed || d.CompletionReason != ReasonExecutionFailed {
		t.Fatalf("status/reason = %s/%s, want %s/%s", d.Status, d.CompletionReason, StatusFailed, ReasonExecutionFailed)
	}
	if got, want := len(d.FailedExecutionIDs), 2; got != want {
		t.Fatalf("failed execution ids = %v, want %d", d.FailedExecutionIDs, want)
	}
}

func TestDecideRecoversCurrentTurnExecutionIDsFromPersistence(t *testing.T) {
	db := newDecisionTestDB(t)
	conv, err := db.CreateConversation("test", database.ConversationCreateMeta{})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	msg, err := db.AddMessage(conv.ID, "assistant", "处理中...", nil)
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	saveDecisionTestExecution(t, db, "run-recovered", mcp.ToolExecutionStatusFailed, conv.ID)

	d := Decide(db, Input{
		Response:                 "工具执行失败。",
		ConversationID:           conv.ID,
		AssistantMessageID:       msg.ID,
		RequireExecutionEvidence: true,
	})

	if d.CompletionReason != ReasonExecutionFailed || d.Status != StatusFailed {
		t.Fatalf("recovered failed execution was not classified correctly: %+v", d)
	}
	if len(d.MCPExecutionIDs) != 1 || d.MCPExecutionIDs[0] != "run-recovered" {
		t.Fatalf("recovered execution ids = %v", d.MCPExecutionIDs)
	}
	if len(d.EvidenceRefs) != 1 || len(d.FailedExecutionIDs) != 1 {
		t.Fatalf("recovered evidence metadata mismatch: %+v", d)
	}
}

func TestDecideDoesNotRecoverExecutionFromEarlierTurn(t *testing.T) {
	db := newDecisionTestDB(t)
	conv, err := db.CreateConversation("test", database.ConversationCreateMeta{})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	saveDecisionTestExecution(t, db, "run-earlier", mcp.ToolExecutionStatusCompleted, conv.ID)
	time.Sleep(2 * time.Millisecond)
	msg, err := db.AddMessage(conv.ID, "assistant", "处理中...", nil)
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}

	d := Decide(db, Input{
		Response:                 "没有执行新工具。",
		ConversationID:           conv.ID,
		AssistantMessageID:       msg.ID,
		RequireExecutionEvidence: true,
	})

	if d.CompletionReason != ReasonMissingEvidence || len(d.MCPExecutionIDs) != 0 {
		t.Fatalf("earlier execution must not satisfy the current turn: %+v", d)
	}
}

func TestDecideFinalizesCompletedEvidence(t *testing.T) {
	db := newDecisionTestDB(t)
	saveDecisionTestExecution(t, db, "run-ok", mcp.ToolExecutionStatusCompleted)

	d := Decide(db, Input{
		Response:                 "任务已处理完成，见工具执行记录。",
		MCPExecutionIDs:          []string{"run-ok"},
		RequireExecutionEvidence: true,
	})

	if !d.Finalizable || !d.Finalized || d.Status != StatusCompleted {
		t.Fatalf("completed execution should finalize: %+v", d)
	}
	if !d.EvidenceVerified || len(d.EvidenceRefs) != 1 {
		t.Fatalf("evidence refs mismatch: %+v", d)
	}
}

func TestDecideAllowsInformationalAnswerWhenExecutionEvidenceIsNotRequired(t *testing.T) {
	d := Decide(nil, Input{Response: "这是一个概念解释，不需要执行工具。"})
	if !d.Finalizable || !d.Finalized || d.Status != StatusCompleted {
		t.Fatalf("informational response should finalize when execution evidence is not required: %+v", d)
	}
}

func TestFromRunResultDoesNotReusePreviousFinalizationStatusAsRunStatus(t *testing.T) {
	db := newDecisionTestDB(t)
	saveDecisionTestExecution(t, db, "run-slow", mcp.ToolExecutionStatusRunning)
	result := &multiagent.RunResult{
		Response:        "工具已触发，按用户要求直接总结。",
		MCPExecutionIDs: []string{"run-slow"},
	}

	first := FromRunResult(db, result, Input{})
	if first.Finalizable || first.CompletionReason != ReasonPendingTools || result.Status != StatusInProgress {
		t.Fatalf("first decision should mark pending and write metadata: decision=%+v result=%+v", first, result)
	}

	saveDecisionTestExecution(t, db, "run-slow", mcp.ToolExecutionStatusCancelled)
	second := FromRunResult(db, result, Input{})
	if !second.Finalizable || !second.Finalized || second.Status != StatusCompleted {
		t.Fatalf("second decision should ignore previous result status after pending cleanup: decision=%+v result=%+v", second, result)
	}
}
