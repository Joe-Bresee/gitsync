package webhook

import (
	"net/http"
	"testing"
)

func TestValidSignatureAcceptsTokenOrHMAC(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-GitSync-Token", "secret")
	if !validSignature(req, []byte("secret"), nil) {
		t.Fatal("expected token signature")
	}
}
