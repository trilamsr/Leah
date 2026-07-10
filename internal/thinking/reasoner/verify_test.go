package reasoner

import (
	"context"
	"strings"
	"testing"
)

func TestVerifyKey_EmptyKey(t *testing.T) {
	err := VerifyKey(context.Background(), "")
	if err == nil {
		t.Fatal("empty key must error")
	}
	if !strings.Contains(err.Error(), "empty key") {
		t.Fatalf("error should name the failure mode; got %v", err)
	}
}
