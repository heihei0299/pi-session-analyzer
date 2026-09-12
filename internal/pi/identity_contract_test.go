package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type piIdentityContractCase struct {
	Name  string                   `json:"name"`
	Same  bool                     `json:"same"`
	Left  piIdentityContractRecord `json:"left"`
	Right piIdentityContractRecord `json:"right"`
}

type piIdentityContractRecord struct {
	Entry   map[string]interface{} `json:"entry"`
	Kind    string                 `json:"kind"`
	Usage   map[string]interface{} `json:"usage"`
	Message map[string]interface{} `json:"message"`
}

func loadPiIdentityContract(t *testing.T) []piIdentityContractCase {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate shared identity contract fixture")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "../../testdata/pi-identity-contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []piIdentityContractCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func TestPiIdentityMatchesSharedContract(t *testing.T) {
	for _, test := range loadPiIdentityContract(t) {
		left := PiRequestIdentity(test.Left.Entry, test.Left.Kind, test.Left.Usage, test.Left.Message)
		right := PiRequestIdentity(test.Right.Entry, test.Right.Kind, test.Right.Usage, test.Right.Message)
		if got := left.SemanticID == right.SemanticID; got != test.Same {
			t.Errorf("%s: semantic equality = %v, want %v", test.Name, got, test.Same)
		}
	}
}
