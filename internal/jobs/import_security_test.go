package jobs

import (
	"context"
	"errors"
	"net"
	"testing"
)

type staticResolver struct {
	addresses []net.IP
	err       error
}

func (r staticResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return r.addresses, r.err
}

func TestImportResolutionRejectsRebinding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rebinding := staticResolver{addresses: []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("127.0.0.1")}}
	if _, err := resolveImportURL(ctx, rebinding, "https://git.example/repo.git"); err != ErrInvalidURL {
		t.Fatalf("rebinding error %v", err)
	}
	public := staticResolver{addresses: []net.IP{net.ParseIP("2606:4700:4700::1111"), net.ParseIP("93.184.216.34")}}
	target, err := resolveImportURL(ctx, public, "https://git.example/repo.git")
	if err != nil || target.address != "93.184.216.34" {
		t.Fatalf("target %+v %v", target, err)
	}
	failed := staticResolver{err: errors.New("dns failed")}
	if _, err := resolveImportURL(ctx, failed, "https://git.example/repo.git"); err != ErrUpstream {
		t.Fatalf("dns error %v", err)
	}
	for _, address := range []string{
		"100.64.0.1", "198.18.0.1", "224.0.0.1", "240.0.0.1",
		"2001:db8::1", "ff02::1", "fec0::1",
	} {
		resolver := staticResolver{addresses: []net.IP{net.ParseIP(address)}}
		if _, err := resolveImportURL(ctx, resolver, "https://git.example/repo.git"); err != ErrInvalidURL {
			t.Fatalf("accepted DNS result %s: %v", address, err)
		}
	}
}

func TestImportErrorCodes(t *testing.T) {
	t.Parallel()
	for _, err := range []error{context.DeadlineExceeded, errors.New("import limit exceeded: bytes"), errors.New("network down")} {
		if mapCloneErr(err) != ErrUpstream {
			t.Fatalf("upstream mapping %v", err)
		}
	}
	for _, err := range []error{errors.New("fatal: Authentication failed"), errors.New("could not read Username: terminal prompts disabled"), errors.New("HTTP 401")} {
		if mapCloneErr(err) != ErrRemoteAuth {
			t.Fatalf("auth mapping %v", err)
		}
	}
}
