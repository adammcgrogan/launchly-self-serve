package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// The builder/editor forms use simple newline-separated textareas for list
// fields (services, certifications, gallery URLs) and "field|field" lines
// for two-part rows (testimonials, business hours) — the same convention
// the old app used, just parsed into normalized rows instead of stored as
// raw delimited strings.

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

func parseServices(s string) []domain.Service {
	var out []domain.Service
	for i, label := range splitLines(s) {
		out = append(out, domain.Service{Label: label, SortOrder: i})
	}
	return out
}

func parseCertifications(s string) []domain.Certification {
	var out []domain.Certification
	for i, label := range splitLines(s) {
		out = append(out, domain.Certification{Label: label, SortOrder: i})
	}
	return out
}

func parseGallery(s string) []domain.GalleryImage {
	var out []domain.GalleryImage
	for i, url := range splitLines(s) {
		out = append(out, domain.GalleryImage{URL: url, SortOrder: i})
	}
	return out
}

// parseTestimonials parses "Name|Role|Quote" lines — role is optional.
func parseTestimonials(s string) []domain.Testimonial {
	var out []domain.Testimonial
	for i, line := range splitLines(s) {
		parts := strings.SplitN(line, "|", 3)
		t := domain.Testimonial{SortOrder: i}
		switch len(parts) {
		case 1:
			t.Quote = strings.TrimSpace(parts[0])
		case 2:
			t.AuthorName = strings.TrimSpace(parts[0])
			t.Quote = strings.TrimSpace(parts[1])
		default:
			t.AuthorName = strings.TrimSpace(parts[0])
			t.AuthorRole = strings.TrimSpace(parts[1])
			t.Quote = strings.TrimSpace(parts[2])
		}
		if t.Quote != "" {
			out = append(out, t)
		}
	}
	return out
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
	"Europe/London", "Europe/Dublin", "Europe/Paris", "Europe/Madrid", "Europe/Berlin",
	"America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles",
	"Australia/Sydney",
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

func servicesToLines(services []domain.Service) string {
	lines := make([]string, len(services))
	for i, s := range services {
		lines[i] = s.Label
	}
	return strings.Join(lines, "\n")
}

func certificationsToLines(c []domain.Certification) string {
	lines := make([]string, len(c))
	for i, x := range c {
		lines[i] = x.Label
	}
	return strings.Join(lines, "\n")
}

func galleryToLines(g []domain.GalleryImage) string {
	lines := make([]string, len(g))
	for i, x := range g {
		lines[i] = x.URL
	}
	return strings.Join(lines, "\n")
}

func testimonialsToLines(t []domain.Testimonial) string {
	lines := make([]string, len(t))
	for i, x := range t {
		lines[i] = fmt.Sprintf("%s|%s|%s", x.AuthorName, x.AuthorRole, x.Quote)
	}
	return strings.Join(lines, "\n")
}

