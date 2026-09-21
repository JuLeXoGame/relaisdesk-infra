package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	domain := flag.String("domain", env("EMAIL_DOMAIN", "relaisdesk.fr"), "domaine d'envoi")
	selector := flag.String("selector", env("SMTP_DKIM_SELECTOR", ""), "sélecteur DKIM")
	strict := flag.Bool("strict", true, "exiger DMARC quarantine/reject")
	flag.Parse()
	if err := checkDNS(*domain, *selector, *strict, net.LookupTXT); err != nil {
		fmt.Fprintln(os.Stderr, "Échec authentification e-mail:", err)
		os.Exit(1)
	}
	fmt.Printf("SPF, DKIM et DMARC sont publiés pour %s.\n", strings.ToLower(strings.TrimSpace(*domain)))
}

func checkDNS(domain, selector string, strict bool, lookup func(string) ([]string, error)) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	selector = strings.TrimSpace(selector)
	if domain == "" || strings.ContainsAny(domain, " /:@") || selector == "" || strings.ContainsAny(selector, " ./:@") {
		return fmt.Errorf("domaine ou sélecteur DKIM invalide")
	}
	spf, err := lookup(domain)
	if err != nil || countPrefixRecords(spf, "v=spf1") != 1 {
		return fmt.Errorf("un unique enregistrement SPF est requis sur %s", domain)
	}
	dkimName := selector + "._domainkey." + domain
	dkim, err := lookup(dkimName)
	if err != nil || !hasNonEmptyTag(dkim, "p") {
		return fmt.Errorf("clé DKIM absente sur %s", dkimName)
	}
	dmarc, err := lookup("_dmarc." + domain)
	if err != nil || countPrefixRecords(dmarc, "v=dmarc1") != 1 {
		return fmt.Errorf("une unique politique DMARC est requise sur _dmarc.%s", domain)
	}
	if strict && !hasTagValue(dmarc, "p", "quarantine", "reject") {
		return fmt.Errorf("DMARC doit utiliser p=quarantine ou p=reject en mode strict")
	}
	return nil
}

func countPrefixRecords(records []string, prefix string) int {
	prefix = strings.ToLower(prefix)
	count := 0
	for _, record := range records {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(record)), prefix) {
			count++
		}
	}
	return count
}

func hasNonEmptyTag(records []string, name string) bool {
	for _, record := range records {
		for _, part := range strings.Split(record, ";") {
			keyValue := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(keyValue) == 2 && strings.EqualFold(strings.TrimSpace(keyValue[0]), name) && strings.TrimSpace(keyValue[1]) != "" {
				return true
			}
		}
	}
	return false
}

func hasTagValue(records []string, name string, values ...string) bool {
	for _, record := range records {
		for _, part := range strings.Split(record, ";") {
			keyValue := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(keyValue) != 2 || !strings.EqualFold(strings.TrimSpace(keyValue[0]), name) {
				continue
			}
			for _, value := range values {
				if strings.EqualFold(strings.TrimSpace(keyValue[1]), value) {
					return true
				}
			}
		}
	}
	return false
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
