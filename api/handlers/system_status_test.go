package handlers

import (
	"context"
	dbpkg "database"
	"net"
	"path/filepath"
	"testing"
	"time"

	"api/mailer"
)

func TestProbeSystemComponents(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "status.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	hbbs := listenForSystemStatusTest(t)
	defer hbbs.Close()
	hbbr := listenForSystemStatusTest(t)
	defer hbbr.Close()
	smtp := listenForSystemStatusTest(t)
	defer smtp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	components := probeSystemComponents(ctx, db, ServerSettings{
		ServerIP: "127.0.0.1", RendezvousPort: hbbs.Addr().(*net.TCPAddr).Port, RelayPort: hbbr.Addr().(*net.TCPAddr).Port,
	}, mailer.NewMailer("127.0.0.1", smtp.Addr().(*net.TCPAddr).Port, "", "", "RelaisDesk <test@example.com>"), true)

	if len(components) != 5 {
		t.Fatalf("components = %d, want 5", len(components))
	}
	for _, component := range components {
		if component.Name != "smtp" && component.Status != "healthy" {
			t.Fatalf("component %s is %s: %s", component.Name, component.Status, component.Message)
		}
		if component.Name == "smtp" && component.Status != "unhealthy" {
			t.Fatalf("SMTP on a non-production test port should be reported unhealthy")
		}
	}
}

func TestProbeSystemComponentsReportsUnavailableSignerAndSMTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	components := probeSystemComponents(ctx, nil, ServerSettings{ServerIP: "127.0.0.1", RendezvousPort: 1, RelayPort: 1}, nil, false)
	unhealthy := 0
	for _, component := range components {
		if component.Status == "unhealthy" {
			unhealthy++
		}
	}
	if unhealthy < 3 {
		t.Fatalf("unhealthy components = %d, want at least 3", unhealthy)
	}
}

func listenForSystemStatusTest(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}
