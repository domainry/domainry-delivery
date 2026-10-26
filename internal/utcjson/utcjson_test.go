package utcjson

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestNestedDeliveryTimesUseUTCMilliseconds(t *testing.T) {
	type record struct {
		CreatedAt  time.Time `json:"created_at"`
		TargetDate string    `json:"target_date"`
	}
	type document struct {
		Record  record   `json:"record"`
		History []record `json:"history"`
	}
	instant := time.Date(2026, time.September, 25, 3, 4, 5, 123000000, time.FixedZone("UTC+8", 8*60*60))
	input := document{Record: record{CreatedAt: instant, TargetDate: "2026-09-25"}, History: []record{{CreatedAt: instant}}}
	raw, err := Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]any
	if err := json.Unmarshal(raw, &encoded); err != nil {
		t.Fatal(err)
	}
	got := encoded["record"].(map[string]any)
	if got["created_at"] != float64(instant.UnixMilli()) || got["target_date"] != "2026-09-25" {
		t.Fatalf("encoded record = %#v", got)
	}
	var decoded document
	if err := Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Record.CreatedAt.Equal(instant) || !decoded.History[0].CreatedAt.Equal(instant) || decoded.Record.TargetDate != input.Record.TargetDate {
		t.Fatalf("decoded document = %#v", decoded)
	}
}

func TestCommandInputRejectsStringTimestamps(t *testing.T) {
	if _, err := NormalizeInput([]byte(`{"occurred_at":"2026-09-25T03:04:05Z"}`)); err == nil {
		t.Fatal("string timestamp was accepted")
	}
	if _, err := NormalizeInput([]byte(`{"occurred_at":1789787045000}`)); err != nil {
		t.Fatalf("numeric timestamp was rejected: %v", err)
	}
}

func TestCommandInputPreservesModelFieldDefinitionsEndingInAt(t *testing.T) {
	raw := []byte(`{"model":{"objects":{"inspection":{"fields":{"inspected_at":"datetime!"}}}}}`)
	normalized, err := NormalizeInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(normalized, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{
		"model": map[string]any{
			"objects": map[string]any{
				"inspection": map[string]any{
					"fields": map[string]any{"inspected_at": "datetime!"},
				},
			},
		},
	}) {
		t.Fatalf("normalized model = %#v", got)
	}
}
