package service

import (
	"context"
	"strings"
	"testing"
)

func TestLeadsAddNoteValidation(t *testing.T) {
	l := &Leads{}

	if _, err := l.AddNote(context.Background(), 1, 1, "   "); err == nil {
		t.Error("AddNote with blank body: want error, got nil")
	}

	tooLong := strings.Repeat("a", maxLongField+1)
	if _, err := l.AddNote(context.Background(), 1, 1, tooLong); err == nil {
		t.Error("AddNote with over-long body: want error, got nil")
	}
}
