package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoCodecPreferenceDefaultAuto(t *testing.T) {
	t.Setenv(EnvDisableHWCodec, "")
	if got := videoCodecPreference(); got != CodecPreferenceAuto {
		t.Fatalf("défaut = %q, want %q", got, CodecPreferenceAuto)
	}
}

func TestVideoCodecPreferenceDisabled(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "y", "on", " 1 "} {
		t.Setenv(EnvDisableHWCodec, v)
		if got := videoCodecPreference(); got != CodecPreferenceSoftware {
			t.Errorf("env=%q: préférence = %q, want %q", v, got, CodecPreferenceSoftware)
		}
	}
	for _, v := range []string{"0", "false", "no", "off", "whatever"} {
		t.Setenv(EnvDisableHWCodec, v)
		if got := videoCodecPreference(); got != CodecPreferenceAuto {
			t.Errorf("env=%q: préférence = %q, want %q", v, got, CodecPreferenceAuto)
		}
	}
}

func TestVideoCodecTomlLines(t *testing.T) {
	t.Setenv(EnvDisableHWCodec, "")
	lf := videoCodecTomlLines(false)
	for _, want := range []string{"codec-preference = 'auto'\n", "av1-test = 'N'\n", "enable-hwcodec = 'Y'\n"} {
		if !strings.Contains(lf, want) {
			t.Errorf("lignes LF: %q absent de %q", want, lf)
		}
	}
	if strings.Contains(lf, "\r") {
		t.Errorf("lignes LF contiennent \\r: %q", lf)
	}
	crlf := videoCodecTomlLines(true)
	for _, want := range []string{"codec-preference = 'auto'\r\n", "av1-test = 'N'\r\n", "enable-hwcodec = 'Y'\r\n"} {
		if !strings.Contains(crlf, want) {
			t.Errorf("lignes CRLF: %q absent de %q", want, crlf)
		}
	}

	t.Setenv(EnvDisableHWCodec, "1")
	off := videoCodecTomlLines(false)
	for _, want := range []string{"codec-preference = 'vp9'\n", "av1-test = 'N'\n", "enable-hwcodec = 'N'\n"} {
		if !strings.Contains(off, want) {
			t.Errorf("désactivé: %q absent de %q", want, off)
		}
	}
}

func TestSetHwCodecInToml(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		enable  bool
		want    []string
		notWant []string
	}{
		{
			name:   "LF active le preset",
			in:     "rendezvous_server = 'x'\n[options]\ncodec-preference = 'vp9'\nenable-hwcodec = 'N'\nkey = 'k'\n",
			enable: true,
			want:   []string{"codec-preference = 'auto'\n", "enable-hwcodec = 'Y'\n", "av1-test = 'N'\n", "key = 'k'\n"},
		},
		{
			name:   "LF force le logiciel",
			in:     "[options]\ncodec-preference = 'auto'\nenable-hwcodec = 'Y'\n",
			enable: false,
			want:   []string{"codec-preference = 'vp9'\n", "enable-hwcodec = 'N'\n", "av1-test = 'N'\n"},
		},
		{
			name:    "CRLF préservé sans doublon",
			in:      "[options]\r\ncodec-preference = 'vp9'\r\ncustom = 'x'\r\n",
			enable:  true,
			want:    []string{"codec-preference = 'auto'\r\n", "custom = 'x'\r\n"},
			notWant: []string{"\r\r", "vp9"},
		},
		{
			name:   "clés manquantes insérées sous options",
			in:     "serial = 0\n[options]\nkey = 'k'\n[autre]\nx = 1\n",
			enable: true,
			want:   []string{"[options]\ncodec-preference = 'auto'\n", "key = 'k'\n", "[autre]\nx = 1\n"},
		},
		{
			name:   "section options créée si absente",
			in:     "serial = 0\n",
			enable: false,
			want:   []string{"serial = 0\n[options]\ncodec-preference = 'vp9'\n", "enable-hwcodec = 'N'\n"},
		},
		{
			name:   "commentaires ignorés",
			in:     "[options]\n#codec-preference = 'vp9'\n",
			enable: true,
			want:   []string{"#codec-preference = 'vp9'\n", "\ncodec-preference = 'auto'\n"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := setHwCodecInToml(tc.in, tc.enable)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("absent %q dans %q", w, got)
				}
			}
			for _, nw := range tc.notWant {
				if strings.Contains(got, nw) {
					t.Errorf("indésirable %q dans %q", nw, got)
				}
			}
			// Idempotence.
			if again := setHwCodecInToml(got, tc.enable); again != got {
				t.Errorf("non idempotent:\n%q\nvs\n%q", got, again)
			}
		})
	}
}

func TestSetHwCodecInFiles(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "RustDesk2.toml")
	p2 := filepath.Join(dir, "sous", "RustDesk2.toml")
	if err := os.MkdirAll(filepath.Dir(p2), 0700); err != nil {
		t.Fatal(err)
	}
	before := "[options]\ncodec-preference = 'vp9'\n"
	if err := os.WriteFile(p1, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	n, err := setHwCodecInFiles([]string{p1, filepath.Join(dir, "absent.toml"), p2}, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("patchés = %d, want 2", n)
	}
	for _, p := range []string{p1, p2} {
		data, _ := os.ReadFile(p)
		if !strings.Contains(string(data), "codec-preference = 'auto'") {
			t.Errorf("%s non patché: %q", p, data)
		}
	}
	if _, err := setHwCodecInFiles([]string{filepath.Join(dir, "rien.toml")}, true); err == nil {
		t.Fatal("erreur attendue quand aucun fichier")
	}
}
