package service

import (
	"context"
	"errors"
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

// TestValidateLeadInput covers the contact form's server-side field
// validation: format checks (email/phone) and per-field length limits. Name
// itself has no format check here — submitLeadForSite (internal/web/site.go)
// rejects a blank name before ever calling into the service.
func TestValidateLeadInput(t *testing.T) {
	tooLongShort := strings.Repeat("a", maxShortField+1)
	tooLongMessage := strings.Repeat("a", maxLongField+1)

	tests := []struct {
		name                                                                    string
		leadName, email, phone, message, serviceLabel, preferredTime, partySize string
		wantErr                                                                 bool
	}{
		{
			name: "valid full submission", leadName: "Jane Doe", email: "jane@example.com", phone: "+1 555-123-4567",
			message: "Looking to book a table for four.", serviceLabel: "Dinner", preferredTime: "7pm", partySize: "4",
		},
		{
			name: "valid with only name and phone", leadName: "Jane Doe", phone: "+1 555-123-4567",
		},
		{name: "no email or phone", leadName: "Jane Doe", wantErr: true},
		{name: "invalid email format", leadName: "Jane Doe", email: "not-an-email", wantErr: true},
		{name: "invalid phone format", leadName: "Jane Doe", phone: "not-a-phone-number-at-all-#$%", wantErr: true},
		{name: "name too long", leadName: tooLongShort, wantErr: true},
		{name: "email too long", leadName: "Jane Doe", email: strings.Repeat("a", maxMediumField) + "@example.com", wantErr: true},
		{name: "phone too long", leadName: "Jane Doe", phone: tooLongShort, wantErr: true},
		{name: "service label too long", leadName: "Jane Doe", serviceLabel: tooLongShort, wantErr: true},
		{name: "preferred time too long", leadName: "Jane Doe", preferredTime: tooLongShort, wantErr: true},
		{name: "party size too long", leadName: "Jane Doe", partySize: tooLongShort, wantErr: true},
		{name: "message too long", leadName: "Jane Doe", message: tooLongMessage, wantErr: true},
		{name: "medium-length field just at boundary is fine", leadName: "Jane Doe", email: strings.Repeat("a", maxMediumField-len("@e.co")) + "@e.co"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLeadInput(tt.leadName, tt.email, tt.phone, tt.message, tt.serviceLabel, tt.preferredTime, tt.partySize)
			if tt.wantErr && err == nil {
				t.Errorf("validateLeadInput(%+v) = nil, want error", tt)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateLeadInput(%+v) = %v, want nil", tt, err)
			}
		})
	}
}

// TestSubmitLeadRejectsInvalidInputWithoutTouchingStore confirms SubmitLead
// validates before it does anything else — an invalid submission must
// return a *ValidationError without dereferencing the (here nil) store,
// mailer, or SMS client. Regression guard for #251: a future reordering of
// validateLeadInput relative to the DB/notification calls would otherwise
// only surface as a nil-pointer panic in production.
func TestSubmitLeadRejectsInvalidInputWithoutTouchingStore(t *testing.T) {
	l := &Leads{} // zero-value: store/mailer/sms are all nil

	err := l.SubmitLead(context.Background(), 1, "Jane Doe", "not-an-email", "", "", "", "", "", "")
	if err == nil {
		t.Fatal("SubmitLead with invalid email: want error, got nil")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Errorf("SubmitLead error = %v (%T), want *ValidationError", err, err)
	}
}
