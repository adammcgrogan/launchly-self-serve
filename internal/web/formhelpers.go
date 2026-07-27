package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// The builder wizard's certifications/gallery fields still use simple
// newline-separated textareas parsed into normalized rows (see
// parseCertifications, parseGallery below) — everything else (services,
// FAQs, staff, testimonials, and the editor's certifications/areas/gallery)
// uses real repeated fields instead (see parseServiceRows,
// parseTestimonialRows, parseCertificationRows, parseServiceAreaRows,
// parseGalleryRows) rather than asking for a delimited line.

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// formRowValues gathers the i-th value of each named field from a repeated
// form-row group, defaulting to "" for a field whose submitted slice is
// shorter than others. When trim is true (parsing for storage) each value is
// whitespace-trimmed; when false (rebuilding a failed submit's form) values
// are passed through exactly as typed, so nothing the user typed is altered
// on reload.
func formRowValues(values url.Values, fields []string, i int, trim bool) []string {
	out := make([]string, len(fields))
	for j, f := range fields {
		vs := values[f]
		if i >= len(vs) {
			continue
		}
		if trim {
			out[j] = strings.TrimSpace(vs[i])
		} else {
			out[j] = vs[i]
		}
	}
	return out
}

// parseRepeatedRows reads a repeatable form-row group for storage: fields[0]
// names the field whose submitted count drives how many rows are considered
// and whose (trimmed) value build treats as required — rows where build
// returns false are dropped, and every retained row's index in the output
// becomes its SortOrder via sortOrder.
func parseRepeatedRows[T any](r *http.Request, fields []string, build func(vals []string, sortOrder int) (T, bool)) []T {
	var out []T
	for i := 0; i < len(r.Form[fields[0]]); i++ {
		if row, ok := build(formRowValues(r.Form, fields, i, true), len(out)); ok {
			out = append(out, row)
		}
	}
	return out
}

// repeatedRowsForForm rebuilds a repeatable form-row group from a failed
// submit's form values, so nothing typed is lost on reload — every row is
// kept (no required-field drop), and it always returns at least one
// (possibly empty) row so the form has one to render.
func repeatedRowsForForm[T any](values url.Values, fields []string, build func(vals []string) T) []T {
	n := 0
	for _, f := range fields {
		if l := len(values[f]); l > n {
			n = l
		}
	}
	rows := make([]T, n)
	for i := range rows {
		rows[i] = build(formRowValues(values, fields, i, false))
	}
	if len(rows) == 0 {
		rows = append(rows, build(make([]string, len(fields))))
	}
	return rows
}

// rowsForDisplay adapts a site's stored rows to a repeatable card form,
// always returning at least one (possibly empty) row so the edit form has
// one to render.
func rowsForDisplay[T any](rows []T) []T {
	if len(rows) == 0 {
		var zero T
		return []T{zero}
	}
	return rows
}

// parseServiceRows reads the repeatable service-menu cards — service_label,
// service_price, and service_description are each submitted as one value
// per row, in the same order, so the i-th value of each names one row. Rows
// with no label are dropped.
func parseServiceRows(r *http.Request) []domain.Service {
	fields := []string{"service_label", "service_price", "service_description"}
	return parseRepeatedRows(r, fields, func(v []string, sortOrder int) (domain.Service, bool) {
		if v[0] == "" {
			return domain.Service{}, false
		}
		return domain.Service{Label: v[0], PriceText: v[1], Description: v[2], SortOrder: sortOrder}, true
	})
}

// serviceRowsForForm rebuilds the service-menu cards from a failed submit's
// form values (or an existing site's services, for the edit form), so
// nothing typed is lost on reload. Always returns at least one (possibly
// empty) row so the form has one to render.
func serviceRowsForForm(values url.Values) []domain.Service {
	fields := []string{"service_label", "service_price", "service_description"}
	return repeatedRowsForForm(values, fields, func(v []string) domain.Service {
		return domain.Service{Label: v[0], PriceText: v[1], Description: v[2]}
	})
}

// serviceRowsForDisplay adapts a site's stored services to the repeatable
// service-menu card form, always returning at least one (possibly empty)
// row so the edit form has one to render.
func serviceRowsForDisplay(services []domain.Service) []domain.Service {
	return rowsForDisplay(services)
}

// parseFAQRows reads the repeatable FAQ cards — faq_question and faq_answer
// are each submitted as one value per row, in the same order, so the i-th
// value of each names one row. Rows with no question are dropped.
func parseFAQRows(r *http.Request) []domain.FAQItem {
	fields := []string{"faq_question", "faq_answer"}
	return parseRepeatedRows(r, fields, func(v []string, sortOrder int) (domain.FAQItem, bool) {
		if v[0] == "" {
			return domain.FAQItem{}, false
		}
		return domain.FAQItem{Question: v[0], Answer: v[1], SortOrder: sortOrder}, true
	})
}

// faqRowsForDisplay adapts a site's stored FAQ items to the repeatable FAQ
// card form, always returning at least one (possibly empty) row so the edit
// form has one to render.
func faqRowsForDisplay(items []domain.FAQItem) []domain.FAQItem {
	return rowsForDisplay(items)
}

// parseStaffRows reads the repeatable staff cards — staff_name, staff_role,
// staff_photo_url, and staff_bio are each submitted as one value per row, in
// the same order, so the i-th value of each names one row. Rows with no name
// are dropped.
func parseStaffRows(r *http.Request) []domain.StaffMember {
	fields := []string{"staff_name", "staff_role", "staff_photo_url", "staff_bio"}
	return parseRepeatedRows(r, fields, func(v []string, sortOrder int) (domain.StaffMember, bool) {
		if v[0] == "" {
			return domain.StaffMember{}, false
		}
		return domain.StaffMember{Name: v[0], Role: v[1], PhotoURL: v[2], Bio: v[3], SortOrder: sortOrder}, true
	})
}

// staffRowsForDisplay adapts a site's stored staff members to the repeatable
// staff card form, always returning at least one (possibly empty) row so the
// edit form has one to render.
func staffRowsForDisplay(members []domain.StaffMember) []domain.StaffMember {
	return rowsForDisplay(members)
}

func parseCertifications(s string) []domain.Certification {
	var out []domain.Certification
	for i, label := range splitLines(s) {
		out = append(out, domain.Certification{Label: label, SortOrder: i})
	}
	return out
}

// parseCertificationRows reads the repeatable certification/trust-badge
// cards — certification_label is submitted once per row. Rows with no label
// are dropped.
func parseCertificationRows(r *http.Request) []domain.Certification {
	fields := []string{"certification_label"}
	return parseRepeatedRows(r, fields, func(v []string, sortOrder int) (domain.Certification, bool) {
		if v[0] == "" {
			return domain.Certification{}, false
		}
		return domain.Certification{Label: v[0], SortOrder: sortOrder}, true
	})
}

// certificationRowsForDisplay adapts a site's stored certifications to the
// repeatable card form, always returning at least one (possibly empty) row
// so the edit form has one to render.
func certificationRowsForDisplay(c []domain.Certification) []domain.Certification {
	return rowsForDisplay(c)
}

// atoiClamp parses a non-negative integer form field, returning 0 for empty
// or malformed input rather than an error — used for optional count fields.
func atoiClamp(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// parseServiceAreaRows reads the repeatable "areas we serve" cards —
// service_area is submitted once per row. Rows with no area are dropped.
func parseServiceAreaRows(r *http.Request) []domain.ServiceArea {
	fields := []string{"service_area"}
	return parseRepeatedRows(r, fields, func(v []string, sortOrder int) (domain.ServiceArea, bool) {
		if v[0] == "" {
			return domain.ServiceArea{}, false
		}
		return domain.ServiceArea{Area: v[0], SortOrder: sortOrder}, true
	})
}

// serviceAreaRowsForDisplay adapts a site's stored service areas to the
// repeatable card form, always returning at least one (possibly empty) row
// so the edit form has one to render.
func serviceAreaRowsForDisplay(a []domain.ServiceArea) []domain.ServiceArea {
	return rowsForDisplay(a)
}

func parseGallery(s string) []domain.GalleryImage {
	var out []domain.GalleryImage
	for i, url := range splitLines(s) {
		out = append(out, domain.GalleryImage{URL: url, SortOrder: i})
	}
	return out
}

// parseGalleryRows reads the repeatable gallery-image cards — gallery_url is
// submitted once per row. Rows with no URL are dropped.
func parseGalleryRows(r *http.Request) []domain.GalleryImage {
	fields := []string{"gallery_url"}
	return parseRepeatedRows(r, fields, func(v []string, sortOrder int) (domain.GalleryImage, bool) {
		if v[0] == "" {
			return domain.GalleryImage{}, false
		}
		return domain.GalleryImage{URL: v[0], SortOrder: sortOrder}, true
	})
}

// galleryRowsForDisplay adapts a site's stored gallery images to the
// repeatable card form, always returning at least one (possibly empty) row
// so the edit form has one to render.
func galleryRowsForDisplay(g []domain.GalleryImage) []domain.GalleryImage {
	return rowsForDisplay(g)
}

// parseTestimonialRows reads the wizard's repeatable testimonial cards —
// testimonial_name/testimonial_role/testimonial_quote are each submitted as
// one value per row, in the same order, so the i-th value of each names one
// row. Rows with no quote are dropped.
func parseTestimonialRows(r *http.Request) []domain.Testimonial {
	fields := []string{"testimonial_quote", "testimonial_name", "testimonial_role"}
	return parseRepeatedRows(r, fields, func(v []string, sortOrder int) (domain.Testimonial, bool) {
		if v[0] == "" {
			return domain.Testimonial{}, false
		}
		return domain.Testimonial{Quote: v[0], AuthorName: v[1], AuthorRole: v[2], SortOrder: sortOrder}, true
	})
}

// testimonialRowsForForm rebuilds the wizard's testimonial cards from a
// failed submit's form values, so nothing typed is lost on reload. Always
// returns at least one (possibly empty) row so the form has one to render.
func testimonialRowsForForm(values url.Values) []domain.Testimonial {
	fields := []string{"testimonial_name", "testimonial_role", "testimonial_quote"}
	return repeatedRowsForForm(values, fields, func(v []string) domain.Testimonial {
		return domain.Testimonial{AuthorName: v[0], AuthorRole: v[1], Quote: v[2]}
	})
}

// testimonialRowsForDisplay adapts a site's stored testimonials to the
// repeatable card form, always returning at least one (possibly empty) row
// so the edit form has one to render.
func testimonialRowsForDisplay(t []domain.Testimonial) []domain.Testimonial {
	return rowsForDisplay(t)
}

// weekdayField describes one row of the opening-hours grid in the builder
// and editor forms: Key is the form-field prefix (hours_<key>_open etc.).
type weekdayField struct {
	Key     string
	Label   string
	Weekday time.Weekday
}

// weekdays drives the opening-hours grid, Monday first (the usual UK
// business convention), independent of time.Weekday's Sunday-first order.
var weekdays = []weekdayField{
	{"mon", "Monday", time.Monday},
	{"tue", "Tuesday", time.Tuesday},
	{"wed", "Wednesday", time.Wednesday},
	{"thu", "Thursday", time.Thursday},
	{"fri", "Friday", time.Friday},
	{"sat", "Saturday", time.Saturday},
	{"sun", "Sunday", time.Sunday},
}

// timezones is a curated list of IANA zones offered in the builder/editor
// timezone select, covering Launchly's UK/Ireland target market plus the
// other zones a small business is most likely to actually need.
var timezones = []string{
	"Europe/London", "Europe/Dublin", "Europe/Paris", "Europe/Madrid", "Europe/Berlin", "Europe/Rome",
	"America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles", "America/Toronto",
	"Australia/Sydney", "Australia/Melbourne", "Pacific/Auckland",
}

// parseBusinessHours reads the 7-day opening-hours grid off the request —
// three fields per day (hours_<key>_open, hours_<key>_close,
// hours_<key>_closed) — skipping any day left entirely blank so hours stay
// optional, same as the old free-text textarea.
func parseBusinessHours(r *http.Request) []domain.BusinessHours {
	var out []domain.BusinessHours
	for _, d := range weekdays {
		closed := r.FormValue("hours_"+d.Key+"_closed") != ""
		opens := strings.TrimSpace(r.FormValue("hours_" + d.Key + "_open"))
		closes := strings.TrimSpace(r.FormValue("hours_" + d.Key + "_close"))
		if !closed && opens == "" && closes == "" {
			continue
		}
		out = append(out, domain.BusinessHours{Weekday: d.Weekday, OpensAt: opens, ClosesAt: closes, Closed: closed})
	}
	return out
}

// parseSpecialHoursRows reads the repeatable special-hours cards —
// special_date, special_label, special_open, special_close, and
// special_closed are each submitted as one value per row, in the same order,
// so the i-th value of each names one row. Rows with no date, or an invalid
// date, are dropped.
func parseSpecialHoursRows(r *http.Request) []domain.SpecialHours {
	dates := r.Form["special_date"]
	labels := r.Form["special_label"]
	opens := r.Form["special_open"]
	closes := r.Form["special_close"]
	closedVals := r.Form["special_closed"]
	var out []domain.SpecialHours
	for i, date := range dates {
		date = strings.TrimSpace(date)
		if date == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		h := domain.SpecialHours{Date: date}
		if i < len(labels) {
			h.Label = strings.TrimSpace(labels[i])
		}
		if i < len(opens) {
			h.OpensAt = strings.TrimSpace(opens[i])
		}
		if i < len(closes) {
			h.ClosesAt = strings.TrimSpace(closes[i])
		}
		if i < len(closedVals) {
			h.Closed = closedVals[i] != ""
		}
		out = append(out, h)
	}
	return out
}

// specialHoursRowsForDisplay adapts a site's stored special-hours overrides
// to the repeatable form, always returning at least one (possibly empty) row
// so the edit form has one to render.
func specialHoursRowsForDisplay(hrs []domain.SpecialHours) []domain.SpecialHours {
	if len(hrs) == 0 {
		return []domain.SpecialHours{{}}
	}
	return hrs
}

// resolveTimezone trims/defaults the submitted timezone field to
// "Europe/London" when blank, so Site.Timezone is never empty.
func resolveTimezone(tz string) string {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return "Europe/London"
	}
	return tz
}

// businessHoursByDay indexes a site's hours by weekday, for pre-filling the
// opening-hours grid on the edit form.
func businessHoursByDay(hrs []domain.BusinessHours) map[time.Weekday]domain.BusinessHours {
	m := make(map[time.Weekday]domain.BusinessHours, len(hrs))
	for _, h := range hrs {
		m[h.Weekday] = h
	}
	return m
}

var socialFields = map[domain.SocialPlatform]string{
	domain.SocialFacebook:  "facebook_url",
	domain.SocialInstagram: "instagram_url",
	domain.SocialWhatsApp:  "whatsapp_url",
	domain.SocialTwitter:   "twitter_url",
	domain.SocialTikTok:    "tiktok_url",
	domain.SocialLinkedIn:  "linkedin_url",
	domain.SocialYouTube:   "youtube_url",
}

func parseSocialLinks(r *http.Request) []domain.SocialLink {
	var out []domain.SocialLink
	for platform, field := range socialFields {
		if v := strings.TrimSpace(r.FormValue(field)); v != "" {
			out = append(out, domain.SocialLink{Platform: platform, URL: v})
		}
	}
	return out
}

// socialLinksMap is used by edit forms to pre-fill each platform's input.
func socialLinksMap(links []domain.SocialLink) map[string]string {
	m := make(map[string]string, len(links))
	for _, l := range links {
		m[string(l.Platform)] = l.URL
	}
	return m
}

// siteContentDisplayData returns the template-data block shared by the
// owner-facing site editor (SiteOverview) and the superadmin site view
// (SuperadminSiteView) — every adapted view of a site's editable content.
// Callers merge this into their own page-specific data map.
func siteContentDisplayData(site *domain.SiteAggregate) map[string]any {
	return map[string]any{
		"Socials":          socialLinksMap(site.SocialLinks),
		"ServiceRows":      serviceRowsForDisplay(site.Services),
		"CertRows":         certificationRowsForDisplay(site.Certifications),
		"AreaRows":         serviceAreaRowsForDisplay(site.ServiceAreas),
		"Reviews":          site.Reviews,
		"TestimonialRows":  testimonialRowsForDisplay(site.Testimonials),
		"GalleryRows":      galleryRowsForDisplay(site.GalleryImages),
		"FAQRows":          faqRowsForDisplay(site.FAQItems),
		"StaffRows":        staffRowsForDisplay(site.StaffMembers),
		"HoursByDay":       businessHoursByDay(site.BusinessHours),
		"SpecialHoursRows": specialHoursRowsForDisplay(site.SpecialHours),
		"Weekdays":         weekdays,
		"Timezones":        timezones,
	}
}
