package session

import (
	"testing"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestWikiPageUsesFromStepsCountsSuccessfulRenderedPagesOnce(t *testing.T) {
	readPages := []map[string]string{
		{"knowledge_base_id": "kb-1", "slug": "entity/yu-zhu", "title": "玉竹"},
		{"knowledge_base_id": "kb-1", "slug": "entity/yu-zhu", "title": "玉竹"},
	}
	steps := types.AgentSteps{{ToolCalls: []types.ToolCall{
		{Name: agenttools.ToolWikiSearch, Result: &types.ToolResult{Success: true, Data: map[string]interface{}{"read_pages": readPages}}},
		{Name: agenttools.ToolWikiReadPage, Result: &types.ToolResult{Success: false, Data: map[string]interface{}{"read_pages": readPages}}},
		{Name: agenttools.ToolWikiReadPage, Result: &types.ToolResult{Success: true, Data: map[string]interface{}{"read_pages": readPages}}},
	}}}

	got := wikiPageUsesFromSteps(steps)
	if len(got) != 1 {
		t.Fatalf("got %d page uses, want one de-duplicated successful read: %+v", len(got), got)
	}
	if got[0].KnowledgeBaseID != "kb-1" || got[0].Slug != "entity/yu-zhu" || got[0].Title != "玉竹" {
		t.Fatalf("unexpected page evidence: %+v", got[0])
	}
}

func TestWikiPageUsesFromStepsAcceptsJSONDecodedData(t *testing.T) {
	steps := types.AgentSteps{{ToolCalls: []types.ToolCall{{
		Name: agenttools.ToolWikiReadPage,
		Result: &types.ToolResult{Success: true, Data: map[string]interface{}{
			"read_pages": []interface{}{map[string]interface{}{
				"knowledge_base_id": "kb-2", "slug": "concept/herbal-food", "title": "药食同源",
			}},
		}},
	}}}}
	got := wikiPageUsesFromSteps(steps)
	if len(got) != 1 || got[0].Slug != "concept/herbal-food" {
		t.Fatalf("JSON-decoded evidence was not extracted: %+v", got)
	}
}
