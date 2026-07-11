package service

import (
	"context"
	"log/slog"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

type Leads struct {
	store  *postgres.Store
	mailer *email.Client
}

func NewLeads(store *postgres.Store, mailer *email.Client) *Leads {
	return &Leads{store: store, mailer: mailer}
}

// SubmitLead records a contact-form submission and forwards it to the
// business owner by email, with the visitor's address set as reply-to.
func (l *Leads) SubmitLead(ctx context.Context, siteID int, name, emailAddr, phone, message string) error {
	lead := &domain.Lead{SiteID: siteID, Name: name, Email: emailAddr, Phone: phone, Message: message}
	if err := postgres.CreateLead(ctx, l.store.DB(), lead); err != nil {
		return err
	}

	site, err := postgres.GetSiteByID(ctx, l.store.DB(), siteID)
	if err != nil || site == nil {
		return err
	}
	contact, err := postgres.GetSiteContact(ctx, l.store.DB(), siteID)
	if err != nil {
		return err
	}
	contactEmail := ""
	if contact != nil {
		contactEmail = contact.Email
	}
	to := notifyEmail(ctx, l.store, site.OwnerUserID, contactEmail)
	if to == "" {
		return nil
	}
	if err := l.mailer.SendLeadNotification(to, site.BusinessName, name, emailAddr, phone, message); err != nil {
		slog.Error("send lead notification", "error", err)
	}
	return nil
}

func (l *Leads) ListBySite(ctx context.Context, siteID int) ([]domain.Lead, error) {
	return postgres.ListLeadsBySite(ctx, l.store.DB(), siteID)
}
