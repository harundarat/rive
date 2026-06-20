package storage

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
)

func TestCanonicalJSONBytesUsesJCS(t *testing.T) {
	data := map[string]any{
		"z": "last",
		"array": []any{
			map[string]any{
				"b": float64(2),
				"a": float64(1),
			},
			map[string]any{
				"d": "second",
				"c": "nested",
			},
		},
		"object": map[string]any{
			"y": true,
			"x": nil,
		},
	}

	got, err := canonicalJSONBytes(data)
	if err != nil {
		t.Fatalf("canonicalJSONBytes returned error: %v", err)
	}

	expected := `{"array":[{"a":1,"b":2},{"c":"nested","d":"second"}],"object":{"x":null,"y":true},"z":"last"}`
	if string(got) != expected {
		t.Fatalf("expected canonical JSON %s, got %s", expected, string(got))
	}
	if strings.ContainsAny(string(got), " \n\t") {
		t.Fatalf("expected no insignificant whitespace, got %s", string(got))
	}
}

func TestCanonicalJSONBytesPreservesWorkOrderNullVerificationHint(t *testing.T) {
	spec := domain.WorkOrderSpec{
		Version:   "1.0",
		ID:        uuid.MustParse("018f95e4-3f8d-7b70-a4dd-2d9a833c4a1f"),
		CreatedAt: "2026-04-22T10:30:00Z",
		Parties: domain.WorkOrderSpecParties{
			Payer: "0xabc",
			Payee: "0xdef",
		},
		Task: domain.WorkOrderSpecTask{
			Title:       "Scrape and clean Yelp reviews for restaurant XYZ",
			Description: "Detailed prose deskripsi tugas...",
			Category:    "data-extraction",
		},
		Deliverable: domain.WorkOrderSpecDeliverable{
			Format: "json",
			Submission: domain.WorkOrderSpecSubmission{
				Method:   "http-callback",
				Endpoint: "https://payee.example/deliver",
			},
		},
		AcceptanceCriteria: []domain.WorkOrderSpecAcceptanceCriteria{
			{
				ID:               "ac1",
				Description:      "Output is valid JSON with >= 100 review objects",
				VerificationHint: nil,
			},
		},
		Compensation: domain.WorkOrderSpecCompensation{
			Amount: "10000000000000000000",
			Asset:  "rUSD",
			Chain:  "mainnet",
		},
		Deadline: "2026-04-23T10:30:00Z",
	}

	got, err := canonicalJSONBytes(spec)
	if err != nil {
		t.Fatalf("canonicalJSONBytes returned error: %v", err)
	}

	if len(got) == 0 {
		t.Fatal("expected non-empty canonical JSON output")
	}
	if !strings.Contains(string(got), `"verificationHint":null`) {
		t.Fatalf("expected null verificationHint to be preserved, got %s", string(got))
	}
}
