package service

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/grycap/oscar/v4/pkg/types"
)

func ReadEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading env file %s: %w", path, err)
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid env file %s at line %d: expected KEY=value", path, lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid env file %s at line %d: empty key", path, lineNumber)
		}

		values[key] = cleanEnvValue(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading env file %s: %w", path, err)
	}
	return values, nil
}

func cleanEnvValue(value string) string {
	if len(value) < 2 {
		return value
	}

	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
	}

	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}

	return value
}

func ApplyEnvFileValuesToService(svc *types.Service, values map[string]string) {
	if svc == nil || len(values) == 0 {
		return
	}

	for key, value := range values {
		if svc.Environment.Vars != nil {
			if _, ok := svc.Environment.Vars[key]; ok {
				svc.Environment.Vars[key] = value
			}
		}
		if svc.Environment.Secrets != nil {
			if _, ok := svc.Environment.Secrets[key]; ok {
				svc.Environment.Secrets[key] = value
			}
		}
	}
}
