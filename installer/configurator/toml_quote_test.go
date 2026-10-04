package main

import (
	"strings"
	"testing"
)

// TestTomlStringRoundTrip replays representative values through a strict
// reading of the TOML string rules: literal output must contain no escape
// processing, basic output must decode back to the input.
func TestTomlStringRoundTrip(t *testing.T) {
	cases := []string{
		"plain",
		`C:\Users\test\AppData`,
		`\\serveur\partage`,
		"https://api.relaisdesk.fr",
		`qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74=`,
		"l'outil",
		`mélangé"\chev/in`,
		"tab\tséparateur",
		"ligne1\nligne2",
		"retour\rchariot",
		"contrôle\x01fin",
		"",
	}
	for _, input := range cases {
		got := tomlString(input)
		if !strings.HasPrefix(got, "'") && !strings.HasPrefix(got, `"`) {
			t.Errorf("tomlString(%q) = %q, not a TOML string", input, got)
			continue
		}
		if strings.HasPrefix(got, "'") {
			inner := got[1 : len(got)-1]
			if inner != input {
				t.Errorf("literal tomlString(%q) = %q, want verbatim", input, got)
			}
			if strings.ContainsAny(inner, "'\n\r") {
				t.Errorf("literal tomlString(%q) = %q, illegal content", input, got)
			}
			continue
		}
		decodeTomlBasicForTest(t, input)
	}
}

// decodeTomlBasicForTest re-decodes a basic string the way the TOML spec
// requires, failing the test on any invalid escape.
func decodeTomlBasicForTest(t *testing.T, input string) string {
	t.Helper()
	src := tomlString(input)
	if !strings.HasPrefix(src, `"`) {
		t.Fatalf("expected basic string for %q, got %q", input, src)
	}
	body := src[1 : len(src)-1]
	var sb strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' {
			sb.WriteByte(c)
			continue
		}
		i++
		if i >= len(body) {
			t.Fatalf("trailing backslash in %q", src)
		}
		switch body[i] {
		case '"':
			sb.WriteByte('"')
		case '\\':
			sb.WriteByte('\\')
		case 'n':
			sb.WriteByte('\n')
		case 'r':
			sb.WriteByte('\r')
		case 't':
			sb.WriteByte('\t')
		case 'u':
			if i+4 >= len(body) {
				t.Fatalf("short \\u escape in %q", src)
			}
			var v int
			for _, h := range body[i+1 : i+5] {
				v *= 16
				switch {
				case h >= '0' && h <= '9':
					v += int(h - '0')
				case h >= 'A' && h <= 'F':
					v += int(h-'A') + 10
				default:
					t.Fatalf("bad \\u escape in %q", src)
				}
			}
			i += 4
			sb.WriteRune(rune(v))
		default:
			t.Fatalf("invalid escape \\%c in %q", body[i], src)
		}
	}
	if sb.String() != input {
		t.Fatalf("basic string %q decodes to %q, want %q", src, sb.String(), input)
	}
	return src
}

func TestTomlStringPrefersLiteral(t *testing.T) {
	if got := tomlString(`C:\dossier\fichier`); got != `'C:\dossier\fichier'` {
		t.Fatalf("got %q, backslashes must stay verbatim in literal strings", got)
	}
	if got := tomlString("simple"); got != "'simple'" {
		t.Fatalf("got %q", got)
	}
	if got := tomlString("l'outil"); !strings.HasPrefix(got, `"`) {
		t.Fatalf("quote must force basic string, got %q", got)
	}
}
