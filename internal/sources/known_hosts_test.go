package source

import "testing"

func TestKnownHostsCallbackRejectsMalformedData(t *testing.T) {
	_, err := knownHostsCallback([]byte("not a known_hosts entry"))
	if err == nil {
		t.Fatal("expected malformed known_hosts data to fail")
	}
}
