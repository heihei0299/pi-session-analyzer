package sessiondata

import (
	"testing"

	"github.com/heihei0299/token-analyzer/internal/domain"
)

func TestSortRowsUsesDeterministicTieBreakers(t *testing.T) {
	sessions := []domain.SessionRow{
		{SessionId: "session-b", Timestamp: "2026-09-10T00:00:00Z", Totals: domain.Totals{TotalTokens: 10}},
		{SessionId: "session-a", Timestamp: "2026-09-10T00:00:00Z", Totals: domain.Totals{TotalTokens: 10}},
	}
	SortSessionRows(sessions, "totalTokens", false)
	if got := sessions[0].SessionId + "," + sessions[1].SessionId; got != "session-a,session-b" {
		t.Fatalf("ascending equal metrics must use session id tie-breaker: %s", got)
	}
	SortSessionRows(sessions, "totalTokens", true)
	if got := sessions[0].SessionId + "," + sessions[1].SessionId; got != "session-b,session-a" {
		t.Fatalf("descending equal metrics must reverse only non-zero comparisons: %s", got)
	}

	requests := []domain.RequestRow{
		{SessionId: "session-b", Timestamp: "2026-09-10T00:00:00Z", Model: "m", Totals: domain.Totals{Output: 3}},
		{SessionId: "session-a", Timestamp: "2026-09-10T00:00:00Z", Model: "m", Totals: domain.Totals{Output: 3}},
	}
	SortRequestRows(requests, "output", false)
	if got := requests[0].SessionId + "," + requests[1].SessionId; got != "session-a,session-b" {
		t.Fatalf("request pagination order must be deterministic: %s", got)
	}
	pageOne := requests[:1][0].SessionId
	pageTwo := requests[1:][0].SessionId
	if pageOne != "session-a" || pageTwo != "session-b" {
		t.Fatalf("tie-breaker must keep page boundaries stable: %s/%s", pageOne, pageTwo)
	}
}
