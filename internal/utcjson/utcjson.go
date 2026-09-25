// Package utcjson encodes Delivery timestamps as UTC Unix milliseconds while
// domain state continues to use time.Time for calendar arithmetic.
package utcjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func Marshal(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return transform(raw, false)
}

func Unmarshal(raw []byte, value any) error {
	converted, err := NormalizeInput(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(converted, value)
}

// NormalizeInput converts numeric timestamp fields to time.Time's JSON form
// before domain command payloads are decoded.
func NormalizeInput(raw []byte) ([]byte, error) {
	return transform(raw, true)
}

func transform(raw []byte, fromMilliseconds bool) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	converted, err := walk(value, fromMilliseconds)
	if err != nil {
		return nil, err
	}
	return json.Marshal(converted)
}

func walk(value any, fromMilliseconds bool) (any, error) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if strings.HasSuffix(key, "_at") {
				if fromMilliseconds {
					if number, ok := child.(json.Number); ok {
						milliseconds, err := number.Int64()
						if err != nil {
							return nil, fmt.Errorf("%s must be integer UTC Unix milliseconds: %w", key, err)
						}
						node[key] = time.UnixMilli(milliseconds).UTC().Format(time.RFC3339Nano)
						continue
					}
					if _, ok := child.(string); ok {
						return nil, fmt.Errorf("%s must be integer UTC Unix milliseconds", key)
					}
				} else if encoded, ok := child.(string); ok {
					if instant, err := time.Parse(time.RFC3339Nano, encoded); err == nil {
						node[key] = instant.UnixMilli()
						continue
					}
				}
			}
			converted, err := walk(child, fromMilliseconds)
			if err != nil {
				return nil, err
			}
			node[key] = converted
		}
	case []any:
		for index, child := range node {
			converted, err := walk(child, fromMilliseconds)
			if err != nil {
				return nil, err
			}
			node[index] = converted
		}
	}
	return value, nil
}
