package main

import (
	"fmt"
	"os"
	"strings"
)

func upsertToml(path string, root map[string]string, sections map[string]map[string]string, removeKeys map[string][]string, removeSections []string) error {
	content, _ := os.ReadFile(path)
	lines := []string{}
	if len(content) > 0 {
		lines = strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	}

	for _, sec := range removeSections {
		lines = removeSection(lines, sec)
	}

	lines = upsertSection(lines, "", root, removeKeys[""])
	for section, values := range sections {
		lines = upsertSection(lines, section, values, removeKeys[section])
	}

	output := strings.Join(lines, "\r\n")
	if !strings.HasSuffix(output, "\r\n") {
		output += "\r\n"
	}
	if err := os.WriteFile(path, []byte(output), 0600); err != nil {
		return fmt.Errorf("erreur d'ecriture de la configuration %s: %w", path, err)
	}
	return nil
}

func removeSection(lines []string, section string) []string {
	start, end := sectionBounds(lines, section)
	if start == -1 {
		return lines
	}
	headerIdx := start - 1
	var newLines []string
	newLines = append(newLines, lines[:headerIdx]...)
	newLines = append(newLines, lines[end:]...)
	return newLines
}

func upsertSection(lines []string, section string, values map[string]string, remove []string) []string {
	toRemove := make(map[string]bool)
	for _, k := range remove {
		toRemove[k] = true
	}

	start, end := sectionBounds(lines, section)
	if start == -1 {
		if len(values) == 0 {
			return lines
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		if section != "" {
			lines = append(lines, "["+section+"]")
		}
		for key, value := range values {
			lines = append(lines, tomlLine(key, value))
		}
		return lines
	}

	seen := map[string]bool{}
	newLines := make([]string, 0, len(lines)+len(values))
	newLines = append(newLines, lines[:start]...)

	for i := start; i < end; i++ {
		key := lineKey(lines[i])
		if toRemove[key] {
			continue
		}
		if value, ok := values[key]; ok {
			newLines = append(newLines, tomlLine(key, value))
			seen[key] = true
		} else {
			newLines = append(newLines, lines[i])
		}
	}

	for key, value := range values {
		if !seen[key] {
			newLines = append(newLines, tomlLine(key, value))
		}
	}

	newLines = append(newLines, lines[end:]...)
	return newLines
}

func sectionBounds(lines []string, section string) (int, int) {
	if section == "" {
		start := 0
		end := len(lines)
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "[") {
				end = i
				break
			}
		}
		return start, end
	}

	header := "[" + section + "]"
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			start = i + 1
			continue
		}
		if start != -1 && strings.HasPrefix(trimmed, "[") {
			return start, i
		}
	}
	if start == -1 {
		return -1, -1
	}
	return start, len(lines)
}

func lineKey(line string) string {
	before, _, ok := strings.Cut(line, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(before)
}

func tomlLine(key, value string) string {
	if key == "nat_type" || key == "serial" {
		return fmt.Sprintf("%s = %s", key, value)
	}
	return fmt.Sprintf("%s = '%s'", key, tomlEscape(value))
}

func tomlEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return value
}
