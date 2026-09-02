package domain

import (
	"testing"
)

func TestTotalsCalculation(t *testing.T) {
	tot := EmptyTotals()
	u1 := Usage{
		Input:      100,
		Output:     50,
		CacheRead:  200,
		CacheWrite: 20, // should NOT be counted in totalTokens (ADR-0002)
		Cost:       &Cost{Total: 0.05},
	}
	AddUsage(&tot, u1)
	FinalizeTotals(&tot)

	if tot.Requests != 1 {
		t.Errorf("expected 1 request, got %d", tot.Requests)
	}
	// totalTokens = input (100) + cacheRead (200) + output (50) = 350
	if tot.TotalTokens != 350 {
		t.Errorf("expected 350 totalTokens, got %f", tot.TotalTokens)
	}
	// cacheRate = 200 / (100 + 200) = 200 / 300 = 0.6666...
	expectedRate := 200.0 / 300.0
	if tot.CacheRate < expectedRate-1e-6 || tot.CacheRate > expectedRate+1e-6 {
		t.Errorf("expected %f cacheRate, got %f", expectedRate, tot.CacheRate)
	}
	if tot.Cost != 0.05 {
		t.Errorf("expected 0.05 cost, got %f", tot.Cost)
	}
}
