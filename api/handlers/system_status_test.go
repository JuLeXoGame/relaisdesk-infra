package handlers

import (
	"context"
	dbpkg "database"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	writeBackupManifestFixture(t, time.Now().UTC().Add(-time.Hour))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	components := probeSystemComponents(ctx, db, ServerSettings{
		ServerIP: "127.0.0.1", RendezvousPort: hbbs.Addr().(*net.TCPAddr).Port, RelayPort: hbbr.Addr().(*net.TCPAddr).Port,
	}, mailer.NewMailer("127.0.0.1", smtp.Addr().(*net.TCPAddr).Port, "", "", "RelaisDesk <test@example.com>"), true)

	if len(components) != 6 {
		t.Fatalf("components = %d, want 6", len(components))
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

func TestProbeBackupComponent(t *testing.T) {
	// Missing manifest alerts.
	t.Setenv("BACKUP_DIR", filepath.Join(t.TempDir(), "no-such-dir"))
	if got := probeBackupComponent(); got.Name != "backups" || got.Status != "unhealthy" {
		t.Fatalf("missing manifest = %+v, want unhealthy backups", got)
	}

	// Stale manifest alerts.
	writeBackupManifestFixture(t, time.Now().UTC().Add(-72*time.Hour))
	if got := probeBackupComponent(); got.Status != "unhealthy" {
		t.Fatalf("stale manifest = %+v, want unhealthy", got)
	}

	// Fresh manifest is healthy and leaks no path.
	writeBackupManifestFixture(t, time.Now().UTC().Add(-2*time.Hour))
	got := probeBackupComponent()
	if got.Status != "healthy" {
		t.Fatalf("fresh manifest = %+v, want healthy", got)
	}
	if strings.Contains(got.Message, "/") || strings.Contains(got.Message, ".aesgcm") {
		t.Fatalf("message leaks a path: %q", got.Message)
	}
}

func writeBackupManifestFixture(t *testing.T, latest time.Time) {
	t.Helper()
	dir := t.TempDir()
	payload := `{"generated_at":"` + time.Now().UTC().Format(time.RFC3339) + `","retention_days":30,"backups":[` +
		`{"file":"relaisdesk-20260101T000000Z.db.aesgcm","size":42,"sha256":"abc","created_at":"` + latest.Format(time.RFC3339) + `","verified":true,"mirrored":true},` +
		`{"file":"relaisdesk-20200101T000000Z.db.aesgcm","size":40,"sha256":"def","created_at":"2020-01-01T00:00:00Z","verified":false,"mirrored":false}]}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKUP_DIR", dir)
}

func listenForSystemStatusTest(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}
