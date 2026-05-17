package xhandle_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/thebitmonk/ai_newsletter/internal/sourceadapter/xhandle"
	"github.com/thebitmonk/ai_newsletter/internal/sources"
)

func TestXHandle_StubReturnsEmpty(t *testing.T) {
	items, err := xhandle.New().Fetch(context.Background(), sources.Source{
		ID:         uuid.New(),
		Type:       sources.TypeXHandle,
		Identifier: "karpathy",
	})
	if err != nil {
		t.Fatalf("stub should not error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("stub should return no items, got %d", len(items))
	}
}
