package service

import (
	"context"
	"log/slog"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/notify"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

type Leads struct {
	store  *postgres.Store
	mailer *email.Client
	sms    *notify.SMSClient
}

func NewLeads(store *postgres.Store, mailer *email.Client, sms *notify.SMSClient) *Leads {
	return &Leads{store: store, mailer: mailer, sms: sms}
}

// SubmitLead records a contact-form submission and forwards it to the
// business owner by email, with the visitor's address set as reply-to. It
// also best-effort texts the owner if they've opted into SMS lead alerts.
func (l *Leads) SubmitLead(ctx context.Context, siteID int, name, emailAddr, phone, message, serviceLabel, preferredTime string) error {
	lead := &domain.Lead{SiteID: siteID, Name: name, Email: emailAddr, Phone: phone, Message: message, ServiceLabel: serviceLabel, PreferredTime: preferredTime}
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
	if to != "" {
		if err := l.mailer.SendLeadNotification(to, site.BusinessName, name, emailAddr, phone, message, serviceLabel, preferredTime); err != nil {
			slog.Error("send lead notification", "error", err)
		}
	}

	l.sendSMSAlert(ctx, site, name)
	return nil
}

// sendSMSAlert texts the owner's mobile about a new lead, best-effort and
// non-blocking like the email notification above. Gated on Pro (per-message
// cost) and the owner's own opt-in toggle + mobile number.
func (l *Leads) sendSMSAlert(ctx context.Context, site *domain.Site, visitorName string) {
	if !l.sms.Configured() {
		return
	}
	billing, err := postgres.GetSiteBilling(ctx, l.store.DB(), site.ID)
	if err != nil || billing == nil || billing.Plan != domain.PlanPro {
		return
	}
	settings, err := postgres.GetSiteNotifySettings(ctx, l.store.DB(), site.ID)
	if err != nil || settings == nil || !settings.SMSAlertsEnabled || settings.MobileNumber == "" {
		return
	}
	if err := l.sms.SendLeadAlert(settings.MobileNumber, site.BusinessName, visitorName); err != nil {
		slog.Error("send lead sms alert", "error", err)
	}
}

func (l *Leads) ListBySite(ctx context.Context, siteID int) ([]domain.Lead, error) {
	return postgres.ListLeadsBySite(ctx, l.store.DB(), siteID)
}
