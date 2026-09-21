package main

import "testing"

func TestCheckDNSRequiresSPFDKIMAndStrictDMARC(t *testing.T) {
	records := map[string][]string{
		"relaisdesk.fr":                      {"v=spf1 include:example.test -all"},
		"selector1._domainkey.relaisdesk.fr": {"v=DKIM1; p=abc"},
		"_dmarc.relaisdesk.fr":               {"v=DMARC1; p=reject; rua=mailto:dmarc@relaisdesk.fr"},
	}
	lookup := func(name string) ([]string, error) { return records[name], nil }
	if err := checkDNS("relaisdesk.fr", "selector1", true, lookup); err != nil {
		t.Fatal(err)
	}
	records["_dmarc.relaisdesk.fr"] = []string{"v=DMARC1; p=none"}
	if err := checkDNS("relaisdesk.fr", "selector1", true, lookup); err == nil {
		t.Fatal("p=none accepted in strict mode")
	}
	records["_dmarc.relaisdesk.fr"] = []string{"v=DMARC1; p=none; sp=reject"}
	if err := checkDNS("relaisdesk.fr", "selector1", true, lookup); err == nil {
		t.Fatal("subdomain policy was mistaken for the domain policy")
	}
	records["_dmarc.relaisdesk.fr"] = []string{"v=DMARC1; p=reject"}
	records["relaisdesk.fr"] = []string{"v=spf1 include:first.test -all", "v=spf1 include:second.test -all"}
	if err := checkDNS("relaisdesk.fr", "selector1", true, lookup); err == nil {
		t.Fatal("multiple SPF records were accepted")
	}
}
