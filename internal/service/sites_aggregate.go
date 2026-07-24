package service

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

// GetSiteAggregate loads a site and everything related to it, serving from
// a short-lived in-memory cache when available to spare the public site
// render path its 16-query fan-out on every hit (see #215).
func (s *Sites) GetSiteAggregate(ctx context.Context, id int) (*domain.SiteAggregate, error) {
	if agg, ok := s.cachedAggregate(id); ok {
		return agg, nil
	}
	agg, err := s.loadSiteAggregate(ctx, id)
	if err != nil {
		return nil, err
	}
	if agg != nil {
		s.cacheAggregate(id, agg)
	}
	return agg, nil
}

func (s *Sites) loadSiteAggregate(ctx context.Context, id int) (*domain.SiteAggregate, error) {
	q := s.store.DB()

	site, err := postgres.GetSiteByID(ctx, q, id)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, nil
	}

	var (
		contact        *domain.SiteContact
		billing        *domain.SiteBilling
		analytics      *domain.SiteAnalyticsSettings
		notify         *domain.SiteNotifySettings
		announcement   *domain.SiteAnnouncement
		reviews        *domain.SiteReviews
		socialLinks    []domain.SocialLink
		services       []domain.Service
		certifications []domain.Certification
		testimonials   []domain.Testimonial
		gallery        []domain.GalleryImage
		faqItems       []domain.FAQItem
		staffMembers   []domain.StaffMember
		hours          []domain.BusinessHours
		specialHours   []domain.SpecialHours
		serviceAreas   []domain.ServiceArea
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { contact, err = postgres.GetSiteContact(gctx, q, id); return })
	g.Go(func() (err error) { billing, err = postgres.GetSiteBilling(gctx, q, id); return })
	g.Go(func() (err error) { analytics, err = postgres.GetSiteAnalyticsSettings(gctx, q, id); return })
	g.Go(func() (err error) { notify, err = postgres.GetSiteNotifySettings(gctx, q, id); return })
	g.Go(func() (err error) { announcement, err = postgres.GetSiteAnnouncement(gctx, q, id); return })
	g.Go(func() (err error) { reviews, err = postgres.GetSiteReviews(gctx, q, id); return })
	g.Go(func() (err error) { socialLinks, err = postgres.GetSiteSocialLinks(gctx, q, id); return })
	g.Go(func() (err error) { services, err = postgres.GetSiteServices(gctx, q, id); return })
	g.Go(func() (err error) { certifications, err = postgres.GetSiteCertifications(gctx, q, id); return })
	g.Go(func() (err error) { testimonials, err = postgres.GetSiteTestimonials(gctx, q, id); return })
	g.Go(func() (err error) { gallery, err = postgres.GetSiteGalleryImages(gctx, q, id); return })
	g.Go(func() (err error) { faqItems, err = postgres.GetSiteFAQItems(gctx, q, id); return })
	g.Go(func() (err error) { staffMembers, err = postgres.GetSiteStaffMembers(gctx, q, id); return })
	g.Go(func() (err error) { hours, err = postgres.GetSiteBusinessHours(gctx, q, id); return })
	g.Go(func() (err error) { specialHours, err = postgres.GetSiteSpecialHours(gctx, q, id); return })
	g.Go(func() (err error) { serviceAreas, err = postgres.GetSiteServiceAreas(gctx, q, id); return })
	if err := g.Wait(); err != nil {
		return nil, err
	}

	if billing == nil {
		billing = &domain.SiteBilling{SiteID: id}
	}

	return &domain.SiteAggregate{
		Site:           *site,
		Contact:        *contact,
		Billing:        *billing,
		Analytics:      *analytics,
		Notify:         *notify,
		Announcement:   *announcement,
		Reviews:        *reviews,
		SocialLinks:    socialLinks,
		Services:       services,
		Certifications: certifications,
		Testimonials:   testimonials,
		GalleryImages:  gallery,
		FAQItems:       faqItems,
		StaffMembers:   staffMembers,
		BusinessHours:  hours,
		SpecialHours:   specialHours,
		ServiceAreas:   serviceAreas,
	}, nil
}

// GetSiteAggregateBySlug is used by the public site renderer.
func (s *Sites) GetSiteAggregateBySlug(ctx context.Context, slug string) (*domain.SiteAggregate, error) {
	site, err := postgres.GetSiteBySlug(ctx, s.store.DB(), slug)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, nil
	}
	return s.GetSiteAggregate(ctx, site.ID)
}

// GetSiteAggregateByCustomDomain is used by the public site renderer to
// resolve a Pro site's connected custom domain.
func (s *Sites) GetSiteAggregateByCustomDomain(ctx context.Context, host string) (*domain.SiteAggregate, error) {
	site, err := postgres.GetSiteByCustomDomain(ctx, s.store.DB(), host)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, nil
	}
	return s.GetSiteAggregate(ctx, site.ID)
}

// GetSiteBySlug loads just a site's own row (no related tables) — used
// where a caller only needs the site's ID, slug, or other core fields,
// rather than paying for GetSiteAggregate's full fan-out of queries.
func (s *Sites) GetSiteBySlug(ctx context.Context, slug string) (*domain.Site, error) {
	return postgres.GetSiteBySlug(ctx, s.store.DB(), slug)
}

// GetSiteByID loads just a site's own row by ID — used to resolve the site
// behind a site_members row (e.g. accepting a team invite).
func (s *Sites) GetSiteByID(ctx context.Context, id int) (*domain.Site, error) {
	return postgres.GetSiteByID(ctx, s.store.DB(), id)
}

// GetSiteByCustomDomain is the lightweight counterpart to
// GetSiteAggregateByCustomDomain, for callers that only need to know
// whether a host resolves to a site (or need its core fields), not the
// full aggregate.
func (s *Sites) GetSiteByCustomDomain(ctx context.Context, host string) (*domain.Site, error) {
	return postgres.GetSiteByCustomDomain(ctx, s.store.DB(), host)
}

// GetSiteContact loads just a site's contact row, for the rare caller that
// needs it without the rest of GetSiteAggregate's fan-out.
func (s *Sites) GetSiteContact(ctx context.Context, siteID int) (*domain.SiteContact, error) {
	return postgres.GetSiteContact(ctx, s.store.DB(), siteID)
}

func (s *Sites) ListSitesByOwner(ctx context.Context, ownerID uuid.UUID) ([]domain.Site, error) {
	return postgres.ListSitesByOwner(ctx, s.store.DB(), ownerID)
}

// ListAllSitesFiltered returns page (1-indexed) of all sites, newest first,
// along with the total count of sites (for pagination). Used by the
// superadmin cross-account view.
func (s *Sites) ListAllSitesFiltered(ctx context.Context, page int) ([]domain.SiteWithBilling, int, error) {
	if page < 1 {
		page = 1
	}
	filter := postgres.SiteFilter{
		Limit:  SitesPageSize,
		Offset: (page - 1) * SitesPageSize,
	}
	return postgres.ListAllSitesFiltered(ctx, s.store.DB(), filter)
}

// SitesPageSize is how many sites ListAllSitesFiltered returns per page.
const SitesPageSize = 20

// PlatformStats returns platform-wide site/plan counts for the superadmin
// dashboard's stats view.
func (s *Sites) PlatformStats(ctx context.Context) (domain.PlatformStats, error) {
	return postgres.GetPlatformStats(ctx, s.store.DB())
}

// ListLiveSites is used by the public sitemap.
func (s *Sites) ListLiveSites(ctx context.Context) ([]domain.Site, error) {
	return postgres.ListLiveSites(ctx, s.store.DB())
}
