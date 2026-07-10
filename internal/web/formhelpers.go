package web

import (
	"fmt"
	"net/http"
	"strings"

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

// parseBusinessHours parses "Label|HoursText" lines, e.g. "Mon-Fri|9am-5pm".
func parseBusinessHours(s string) []domain.BusinessHours {
	var out []domain.BusinessHours
	for i, line := range splitLines(s) {
		parts := strings.SplitN(line, "|", 2)
		bh := domain.BusinessHours{SortOrder: i, Label: strings.TrimSpace(parts[0])}
		if len(parts) == 2 {
			bh.HoursText = strings.TrimSpace(parts[1])
		}
		if bh.Label != "" {
			out = append(out, bh)
		}
	}
	return out
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

func businessHoursToLines(hrs []domain.BusinessHours) string {
	lines := make([]string, len(hrs))
	for i, x := range hrs {
		lines[i] = fmt.Sprintf("%s|%s", x.Label, x.HoursText)
	}
	return strings.Join(lines, "\n")
}
