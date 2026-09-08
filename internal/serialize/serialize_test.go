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
