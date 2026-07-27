package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/cloudflare"
	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/lib/pq"
)

// DomainRegistrar is the subset of Cloudflare's custom-hostnames API the
// domain service needs — an interface so tests can fake it without hitting
// Cloudflare for SaaS.
type DomainRegistrar interface {
	CreateCustomHostname(ctx context.Context, hostname string) (*cloudflare.Hostname, error)
	GetCustomHostname(ctx context.Context, cfID string) (*cloudflare.Hostname, error)
	DeleteCustomHostname(ctx context.Context, cfID string) error
}

// domainsMailer is the subset of email.Client's methods domains.go calls.
// Depending on this narrow interface (rather than *email.Client directly)
// lets tests substitute a fake mailer that records calls instead of hitting
// Resend over the network — mirrors billingMailer in billing.go.
type domainsMailer interface {
	SendDomainVerified(to, businessName, customDomain, dashboardURL string) error
	SendDomainFailed(to, businessName, customDomain, dashboardURL string) error
}

// Domains manages Pro-plan custom domains. Each domain is registered with
// Cloudflare for SaaS, which terminates TLS per-hostname and proxies to one
// fixed origin — Railway itself never sees customer domains, so this
// doesn't depend on Railway's own per-service domain limits.
type Domains struct {
	store          *postgres.Store
	cf             DomainRegistrar
	mailer         domainsMailer
	fallbackOrigin string
	platformDomain string
	baseURL        string
}

func NewDomains(store *postgres.Store, cf DomainRegistrar, mailer *email.Client, fallbackOrigin, platformDomain, baseURL string) *Domains {
	return &Domains{store: store, cf: cf, mailer: mailer, fallbackOrigin: fallbackOrigin, platformDomain: platformDomain, baseURL: baseURL}
}

// Errors returned by the custom domain flow — web handlers show these
// directly to the site owner, so their text is user-facing.
var (
	ErrCustomDomainNotPro  = errors.New("custom domains are a Pro-plan feature — upgrade to connect your own domain.")
	ErrCustomDomainInvalid = errors.New("enter a valid domain, e.g. yourbusiness.com.")
	ErrCustomDomainTaken   = errors.New("that domain is already connected to another site.")
)

// FallbackOrigin is the fixed hostname customers CNAME their domain to.
func (d *Domains) FallbackOrigin() string {
	return d.fallbackOrigin
}

var domainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

func normalizeDomain(raw string) string {
	h := strings.ToLower(strings.TrimSpace(raw))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexAny(h, "/:"); i >= 0 {
		h = h[:i]
	}
	return h
}

func validDomain(h string) bool {
	return h != "" && len(h) <= 253 && domainRe.MatchString(h)
}

// SetCustomDomain registers a Pro site's custom domain with Cloudflare and
// records it as pending verification. Re-submitting the same domain (e.g.
// after a failed attempt) is allowed; submitting a domain already attached
// to a different site is not.
func (d *Domains) SetCustomDomain(ctx context.Context, siteID int, rawDomain string) (*cloudflare.Hostname, error) {
	host := normalizeDomain(rawDomain)
	if !validDomain(host) || host == d.platformDomain || strings.HasSuffix(host, "."+d.platformDomain) {
		return nil, ErrCustomDomainInvalid
	}

	tx, err := d.store.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	site, err := postgres.GetSiteByID(ctx, tx, siteID)
	if err != nil {
		return nil, fmt.Errorf("load site: %w", err)
	}
	if site == nil {
		return nil, fmt.Errorf("site %d not found", siteID)
	}
	billing, err := postgres.GetSiteBilling(ctx, tx, siteID)
	if err != nil {
		return nil, fmt.Errorf("load billing: %w", err)
	}
	if billing == nil || !billing.IsPro() {
		return nil, ErrCustomDomainNotPro
	}

	taken, err := postgres.CustomDomainInUse(ctx, tx, host)
	if err != nil {
		return nil, fmt.Errorf("check domain: %w", err)
	}
	if taken && site.CustomDomain != host {
		return nil, ErrCustomDomainTaken
	}

	hostname, err := d.cf.CreateCustomHostname(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("register with cloudflare: %w", err)
	}
	if err := postgres.SetCustomDomain(ctx, tx, siteID, host, hostname.ID); err != nil {
		if isUniqueCustomDomainViolation(err) {
			if delErr := d.cf.DeleteCustomHostname(ctx, hostname.ID); delErr != nil {
				slog.Error("orphaned cloudflare hostname after lost domain-claim race", "cf_id", hostname.ID, "domain", host, "error", delErr)
			}
			return nil, ErrCustomDomainTaken
		}
		return nil, fmt.Errorf("save custom domain: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return hostname, nil
}

// isUniqueCustomDomainViolation reports whether err is a Postgres
// unique-constraint violation (23505) on the sites table's custom_domain
// column — the race two concurrent claims for the same new domain can hit.
func isUniqueCustomDomainViolation(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return pqErr.Code == "23505" && pqErr.Constraint == "sites_custom_domain_key"
}

// RefreshCustomDomainStatus re-checks a site's custom domain against
// Cloudflare and updates its stored verification status accordingly. The
// first time a domain lands on active or failed, the owner is emailed —
// this is the only place that transition is detected, so it covers both
// the lazy refresh on dashboard load and the manual "check status" button
// (#228).
func (d *Domains) RefreshCustomDomainStatus(ctx context.Context, siteID int) (*cloudflare.Hostname, error) {
	site, err := postgres.GetSiteByID(ctx, d.store.DB(), siteID)
	if err != nil {
		return nil, fmt.Errorf("load site: %w", err)
	}
	if site == nil || site.CustomDomainCFID == "" {
		return nil, fmt.Errorf("site %d has no custom domain", siteID)
	}

	hostname, err := d.cf.GetCustomHostname(ctx, site.CustomDomainCFID)
	if err != nil {
		return nil, fmt.Errorf("check cloudflare status: %w", err)
	}

	status := domain.CustomDomainPending
	switch {
	case hostname.Active():
		status = domain.CustomDomainActive
	case hostname.Failed():
		status = domain.CustomDomainFailed
	}
	if err := postgres.UpdateCustomDomainStatus(ctx, d.store.DB(), siteID, status); err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	if status != site.CustomDomainStatus && (status == domain.CustomDomainActive || status == domain.CustomDomainFailed) {
		d.notifyDomainStatusChange(ctx, siteID, site.CustomDomain, status)
	}
	return hostname, nil
}

// notifyDomainStatusChange emails the site owner that their custom domain
// just went live or failed verification. Best-effort: a failed send just
// means the owner finds out from the dashboard instead, same as before
// this existed.
func (d *Domains) notifyDomainStatusChange(ctx context.Context, siteID int, customDomain string, status domain.CustomDomainStatus) {
	site, to, err := resolveNotifyTarget(ctx, d.store, siteID)
	if err != nil || site == nil || to == "" {
		return
	}
	dashboardURL := fmt.Sprintf("%s/dashboard/sites/%s", d.baseURL, site.Slug)
	if status == domain.CustomDomainActive {
		if err := d.mailer.SendDomainVerified(to, site.BusinessName, customDomain, dashboardURL); err != nil {
			slog.Error("send domain verified email", "site_id", siteID, "error", err)
		}
		return
	}
	if err := d.mailer.SendDomainFailed(to, site.BusinessName, customDomain, dashboardURL); err != nil {
		slog.Error("send domain failed email", "site_id", siteID, "error", err)
	}
}

// RemoveCustomDomain detaches a site's custom domain, deleting it from
// Cloudflare first. If it was never registered with Cloudflare (or is
// already gone there), the local record is cleared regardless.
func (d *Domains) RemoveCustomDomain(ctx context.Context, siteID int) error {
	site, err := postgres.GetSiteByID(ctx, d.store.DB(), siteID)
	if err != nil {
		return fmt.Errorf("load site: %w", err)
	}
	if site != nil && site.CustomDomainCFID != "" {
		if err := d.cf.DeleteCustomHostname(ctx, site.CustomDomainCFID); err != nil {
			return fmt.Errorf("remove from cloudflare: %w", err)
		}
	}
	return postgres.ClearCustomDomain(ctx, d.store.DB(), siteID)
}
