package serialize

import (
	"strings"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
)

func TestSerializeCSVPreservesUnpricedStatus(t *testing.T) {
	data, err := SerializeCSV("sessions", []domain.SessionRow{{SessionId: "s", Totals: domain.Totals{CostStatus: "unpriced"}}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "costStatus") || !strings.Contains(text, "unpriced") {
		t.Fatalf("CSV should preserve unpriced status: %s", text)
	}
}

func TestSerializePreservesPartialCostAmountAndStatus(t *testing.T) {
	totals := domain.Totals{Requests: 3, Cost: 0.25, CostStatus: "unpriced"}
	jsonData, err := SerializeJSON(totals)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonData), `"cost": 0.25`) || !strings.Contains(string(jsonData), `"costStatus": "unpriced"`) {
		t.Fatalf("JSON must keep partial cost and status: %s", jsonData)
	}

	csvData, err := SerializeCSV("totals", totals)
	if err != nil {
		t.Fatal(err)
	}
	csvText := string(csvData)
	if !strings.Contains(csvText, "0.2500") || !strings.Contains(csvText, "unpriced") {
		t.Fatalf("CSV must keep partial cost and status: %s", csvText)
	}
}
