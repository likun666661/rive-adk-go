package llmagent

import "testing"

func TestNewRequiresFlow(t *testing.T) {
	_, err := New("missing_flow", "test agent", nil)
	if err == nil {
		t.Fatal("expected error for nil flow")
	}
}
