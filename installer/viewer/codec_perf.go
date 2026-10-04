package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Preset réactivité vidéo RelaisDesk.
//
// Le codec matériel est préféré quand il est disponible (négociation auto du
// core : H265 > H264 > AV1/VP9/VP8). Le core désactive lui-même le matériel en
// cas d'échec (création ou encodage) avec repli logiciel, donc 'auto' est sûr
// par défaut. L'opérateur peut forcer le logiciel à tout moment.
const (
	// CodecPreferenceAuto laisse le core négocier le meilleur codec.
	CodecPreferenceAuto = "auto"
	// CodecPreferenceSoftware force le repli logiciel VP9.
	CodecPreferenceSoftware = "vp9"
	// EnvDisableHWCodec force le logiciel quand il vaut 1/true/yes/on.
	EnvDisableHWCodec = "RELAISDESK_DISABLE_HWCODEC"
)

// hwCodecDisabledByOperator reports whether the operator forced software-only
// video encoding for this process.
func hwCodecDisabledByOperator() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvDisableHWCodec))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// videoCodecPreference returns the codec-preference value to write.
func videoCodecPreference() string {
	if hwCodecDisabledByOperator() {
		return CodecPreferenceSoftware
	}
	return CodecPreferenceAuto
}

// videoCodecTomlLines returns the [options] lines for the reactivity preset.
// AV1 logiciel est toujours désactivé (trop lent en software) ; le matériel
// reste piloté par enable-hwcodec (défaut Y).
func videoCodecTomlLines(crlf bool) string {
	eol := "\n"
	if crlf {
		eol = "\r\n"
	}
	hw := "Y"
	if hwCodecDisabledByOperator() {
		hw = "N"
	}
	return "codec-preference = '" + videoCodecPreference() + "'" + eol +
		"av1-test = 'N'" + eol +
		"enable-hwcodec = '" + hw + "'" + eol
}

// setHwCodecInToml rewrites the codec keys of an existing RustDesk2.toml
// content. enable=true restores the preset, false forces software VP9.
// Only keys under [options] are read or written: the core ignores the same
// names anywhere else, so touching them would report success while changing
// nothing. Line endings and unrelated content are preserved; missing keys are
// inserted right after [options], or appended with a new section when absent.
func setHwCodecInToml(content string, enable bool) string {
	wantCodec := CodecPreferenceAuto
	wantHW := "Y"
	if !enable {
		wantCodec = CodecPreferenceSoftware
		wantHW = "N"
	}
	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	// Work in LF space, then restore CRLF at the end if needed.
	trimmed := strings.TrimSuffix(content, eol)
	lines := strings.Split(trimmed, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	seenCodec, seenHW, seenAV1 := false, false, false
	optIdx := -1
	inOptions := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			inOptions = optionsHeaderName(t) == "options"
			if inOptions && optIdx == -1 {
				optIdx = i
			}
			continue
		}
		if !inOptions {
			continue
		}
		if strings.HasPrefix(t, "#") || !strings.Contains(t, "=") {
			continue
		}
		key := strings.TrimSpace(strings.SplitN(t, "=", 2)[0])
		switch key {
		case "codec-preference":
			lines[i] = "codec-preference = '" + wantCodec + "'"
			seenCodec = true
		case "enable-hwcodec":
			lines[i] = "enable-hwcodec = '" + wantHW + "'"
			seenHW = true
		case "av1-test":
			lines[i] = "av1-test = 'N'"
			seenAV1 = true
		}
	}
	var add []string
	if !seenCodec {
		add = append(add, "codec-preference = '"+wantCodec+"'")
	}
	if !seenAV1 {
		add = append(add, "av1-test = 'N'")
	}
	if !seenHW {
		add = append(add, "enable-hwcodec = '"+wantHW+"'")
	}
	out := strings.Join(lines, "\n")
	if len(add) > 0 {
		block := strings.Join(add, "\n")
		if optIdx >= 0 {
			head := strings.Join(lines[:optIdx+1], "\n")
			tail := strings.Join(lines[optIdx+1:], "\n")
			out = head + "\n" + block
			if tail != "" {
				out += "\n" + tail
			}
		} else {
			if out != "" {
				out += "\n"
			}
			out += "[options]\n" + block
		}
	}
	if eol == "\r\n" {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	if strings.HasSuffix(content, eol) {
		out += eol
	}
	return out
}

// optionsHeaderName returns the section name opened by a trimmed line
// ("[options] # ...", "[ options ]"), or "" when the line opens no section.
// TOML names stay case-sensitive: only exactly "options" counts.
func optionsHeaderName(t string) string {
	if !strings.HasPrefix(t, "[") {
		return ""
	}
	end := strings.Index(t, "]")
	if end <= 0 {
		return ""
	}
	return strings.TrimSpace(t[1:end])
}

// writeFileAtomic replaces path without ever exposing a truncated file: a
// crash between open and close of a direct write would leave a half-written
// TOML the core then fails to parse.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// setHwCodecInFiles patches every existing RustDesk2.toml in paths.
// Missing files are skipped; an error is returned only when nothing was
// patched or a write fails.
func setHwCodecInFiles(paths []string, enable bool) (int, error) {
	patched := 0
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := writeFileAtomic(p, []byte(setHwCodecInToml(string(data), enable)), 0600); err != nil {
			return patched, fmt.Errorf("écriture %s: %w", p, err)
		}
		patched++
	}
	if patched == 0 {
		return 0, fmt.Errorf("aucun RustDesk2.toml trouvé — enrôlez d'abord ce poste")
	}
	return patched, nil
}

// setHwCodecEverywhere applies setHwCodecInFiles to every known config path.
func setHwCodecEverywhere(enable bool) (int, error) {
	return setHwCodecInFiles(rustDesk2TomlPaths(), enable)
}
