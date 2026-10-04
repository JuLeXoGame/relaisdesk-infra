package main

import (
	"fmt"
	"strings"
)

// tomlString renders value as TOML source: a literal string ('...') when the
// value allows it, otherwise a basic string ("...") with the escapes the
// format defines. Literal strings perform no unescaping, so backslashes must
// stay verbatim inside them (doubling them would corrupt Windows paths),
// while a quote or control character forces the basic form.
func tomlString(value string) string {
	basic := false
	for _, r := range value {
		if r == '\'' || r == 0x7f || (r < 0x20 && r != '\t') {
			basic = true
			break
		}
	}
	if !basic {
		return "'" + value + "'"
	}
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&sb, `\u%04X`, r)
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
