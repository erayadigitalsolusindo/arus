package platformadmin

import (
	"context"
	"errors"
	"net"
	"testing"
)

type fakeDNS struct {
	cname map[string]string
	hosts map[string][]string
}

func (f fakeDNS) LookupCNAME(_ context.Context, host string) (string, error) {
	if c, ok := f.cname[host]; ok {
		return c + ".", nil
	}
	return host + ".", nil // tanpa CNAME: nama kanonik = host itu sendiri
}

func (f fakeDNS) LookupHost(_ context.Context, host string) ([]string, error) {
	if ips, ok := f.hosts[host]; ok {
		return ips, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func TestNormalizeHost(t *testing.T) {
	ok := map[string]string{
		"arus.kotakcantik.com":    "arus.kotakcantik.com",
		"  ARUS.KotakCantik.com. ": "arus.kotakcantik.com",
		"a-b.c.co.id":             "a-b.c.co.id",
	}
	for in, want := range ok {
		if got, err := normalizeHost(in); err != nil || got != want {
			t.Errorf("normalizeHost(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "localhost", "-a.com", "a_b.com", "http://a.com", "a.com/x", "a..com", "192.168.0.1", "a.c"} {
		if _, err := normalizeHost(in); !errors.Is(err, ErrDomainInvalid) {
			t.Errorf("normalizeHost(%q) harusnya ditolak, err=%v", in, err)
		}
	}
}

func TestCheckDomainDNS(t *testing.T) {
	const target = "arus.erayadigital.co.id"
	svc := func(d fakeDNS) *Service { return &Service{Deps: Deps{DomainTarget: target}, dns: d} }
	ctx := context.Background()

	t.Run("cname benar", func(t *testing.T) {
		s := svc(fakeDNS{cname: map[string]string{"arus.kotakcantik.com": "ARUS.erayadigital.co.id"}})
		if err := s.checkDomainDNS(ctx, "arus.kotakcantik.com"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("ip sama (proxy/ALIAS)", func(t *testing.T) {
		s := svc(fakeDNS{hosts: map[string][]string{"arus.kotakcantik.com": {"1.2.3.4"}, target: {"1.2.3.4", "5.6.7.8"}}})
		if err := s.checkDomainDNS(ctx, "arus.kotakcantik.com"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("cname salah", func(t *testing.T) {
		s := svc(fakeDNS{
			cname: map[string]string{"arus.kotakcantik.com": "lain.example.com"},
			hosts: map[string][]string{"arus.kotakcantik.com": {"9.9.9.9"}, target: {"1.2.3.4"}},
		})
		if err := s.checkDomainDNS(ctx, "arus.kotakcantik.com"); err == nil {
			t.Fatal("harusnya gagal")
		}
	})
	t.Run("dns belum ada", func(t *testing.T) {
		if err := svc(fakeDNS{}).checkDomainDNS(ctx, "arus.kotakcantik.com"); err == nil {
			t.Fatal("harusnya gagal")
		}
	})
}
