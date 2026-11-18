package platform

import (
	"context"
	"testing"
	"time"
)

func TestPlainServiceDialChecksResolvedAddress(t *testing.T) {
	dialer := ServiceDialer("http://harvest:8080", time.Second, time.Second)
	for _, address := range []string{"127.0.0.1:8080", "172.20.0.2:8080", "[fd00::2]:8080"} {
		if err := dialer.ControlContext(context.Background(), "tcp", address, nil); err != nil {
			t.Errorf("private address rejected: %s", address)
		}
	}
	for _, address := range []string{"203.0.113.1:8080", "169.254.169.254:8080", "not-an-ip:8080"} {
		if err := dialer.ControlContext(context.Background(), "tcp", address, nil); err == nil {
			t.Errorf("plaintext could reach %s", address)
		}
	}
	if ServiceDialer("https://harvest.example", time.Second, time.Second).ControlContext != nil {
		t.Fatal("explicit HTTPS must support normal TLS connections")
	}
}
