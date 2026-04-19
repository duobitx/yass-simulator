package internal

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// GetByPath extracts a value from nested map[string]any or []any
// given a path like "user.addresses[0].street"
func GetByPath[T any](data any, path string) (*T, error) {
	parts := parsePath(path)
	current := data
	for _, part := range parts {
		switch v := current.(type) {
		case map[string]any:
			next, ok := v[part.key]
			if !ok {
				return nil, fmt.Errorf("cannot extract subkey %s", part.key)
			}
			current = next
		case []any:
			if part.index == nil {
				return nil, fmt.Errorf("index required for slice")
			}
			idx := *part.index
			if idx < 0 || idx >= len(v) {
				return nil, fmt.Errorf("slice index out of range %d not in range %d..%d", idx, 0, len(v))
			}
			current = v[idx]

		default:
			return nil, errors.New("cannot descend further")
		}

		if part.index != nil && reflect.TypeOf(current).Kind() == reflect.Slice {
			arr := current.([]any)
			idx := *part.index
			if idx < 0 || idx >= len(arr) {
				return nil, fmt.Errorf("should not happen")
			}
			current = arr[idx]
		}
	}
	t, ok := current.(T)
	if !ok {
		return nil, fmt.Errorf("cannot cast %T", current)
	}
	return &t, nil
}

// pathPart represents one segment of a parsed path, like "addresses[0]"
type pathPart struct {
	key   string
	index *int
}

// parsePath splits "user.addresses[0].street" into parts
func parsePath(path string) []pathPart {
	rawParts := strings.Split(strings.TrimLeft(path, "."), ".")
	parts := make([]pathPart, 0, len(rawParts))

	for _, raw := range rawParts {
		var part pathPart
		if i := strings.Index(raw, "["); i != -1 {
			// e.g. "addresses[0]"
			part.key = raw[:i]
			idxStr := strings.TrimSuffix(raw[i+1:], "]")
			idx, err := strconv.Atoi(idxStr)
			if err == nil {
				part.index = &idx
			}
		} else {
			part.key = raw
		}
		parts = append(parts, part)
	}

	return parts
}
