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
// business owner by email, with the visitor's address set as reply-to. If
// the visitor supplied their own email, it also sends them an instant
// auto-reply confirming receipt. It also best-effort texts the owner if
// they've opted into SMS lead alerts.
func (l *Leads) SubmitLead(ctx context.Context, siteID int, name, emailAddr, phone, message, serviceLabel, preferredTime, siteURL string) error {
	lead := &domain.Lead{SiteID: siteID, Name: name, Email: emailAddr, Phone: phone, Message: message, ServiceLabel: serviceLabel, PreferredTime: preferredTime}
	if err := postgres.CreateLead(ctx, l.store.DB(), lead); err != nil {
		return err
	}
	if err := postgres.RecordSiteEvent(ctx, l.store.DB(), &domain.SiteEvent{SiteID: siteID, Kind: domain.EventKindLead}); err != nil {
		slog.Error("record lead conversion event", "site_id", siteID, "error", err)
	}

	site, err := postgres.GetSiteByID(ctx, l.store.DB(), siteID)
	if err != nil || site == nil {
		return err
	}
	contact, err := postgres.GetSiteContact(ctx, l.store.DB(), siteID)
	if err != nil {
		return err
	}
	contactEmail, contactPhone := "", ""
	if contact != nil {
		contactEmail = contact.Email
		contactPhone = contact.Phone
	}
	to := notifyEmail(ctx, l.store, site.OwnerUserID, contactEmail)
	if to != "" {
		if err := l.mailer.SendLeadNotification(to, site.BusinessName, name, emailAddr, phone, message, serviceLabel, preferredTime); err != nil {
			slog.Error("send lead notification", "error", err)
		}
	}

	if emailAddr != "" {
		hours, err := postgres.GetSiteBusinessHours(ctx, l.store.DB(), siteID)
		if err != nil {
			slog.Error("get site business hours for auto-reply", "error", err)
		}
		if err := l.mailer.SendLeadAutoReply(emailAddr, site.BusinessName, hours, contactPhone, siteURL); err != nil {
			slog.Error("send lead auto-reply", "error", err)
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

// ListBySiteFiltered returns page (1-indexed) of a site's leads narrowed by
// status (empty = all) and a name/email search term, along with the total
// count of leads matching that filter (for pagination).
func (l *Leads) ListBySiteFiltered(ctx context.Context, siteID int, status domain.LeadStatus, search string, page int) ([]domain.Lead, int, error) {
	if page < 1 {
		page = 1
	}
	filter := postgres.LeadFilter{
		Status: status,
		Search: search,
		Limit:  LeadsPageSize,
		Offset: (page - 1) * LeadsPageSize,
	}
	return postgres.ListLeadsBySiteFiltered(ctx, l.store.DB(), siteID, filter)
}

// LeadsPageSize is how many leads ListBySiteFiltered returns per page.
const LeadsPageSize = 20

// Counts returns a site's total and new lead counts, unaffected by any list filter.
func (l *Leads) Counts(ctx context.Context, siteID int) (domain.LeadCounts, error) {
	return postgres.GetLeadCounts(ctx, l.store.DB(), siteID)
}

// UpdateStatus sets a lead's follow-up status, scoped to siteID so an owner
// can't update a lead belonging to a site they don't own.
func (l *Leads) UpdateStatus(ctx context.Context, siteID, leadID int, status domain.LeadStatus) error {
	return postgres.UpdateLeadStatus(ctx, l.store.DB(), siteID, leadID, status)
}
