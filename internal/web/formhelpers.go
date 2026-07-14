package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// The editor form uses simple newline-separated textareas for list fields
// (certifications, gallery URLs) and "field|field" lines for testimonials —
// the same convention the old app used, just parsed into normalized rows
// instead of stored as raw delimited strings. Services and the builder
// wizard's testimonials use real repeated fields instead (see
// parseServiceRows, parseTestimonialRows) rather than asking for a
// delimited line.

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

// parseServiceRows reads the repeatable service-menu cards — service_label,
// service_price, and service_description are each submitted as one value
// per row, in the same order, so the i-th value of each names one row. Rows
// with no label are dropped.
func parseServiceRows(r *http.Request) []domain.Service {
	labels := r.Form["service_label"]
	prices := r.Form["service_price"]
	descriptions := r.Form["service_description"]
	var out []domain.Service
	for i, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		s := domain.Service{Label: label, SortOrder: len(out)}
		if i < len(prices) {
			s.PriceText = strings.TrimSpace(prices[i])
		}
		if i < len(descriptions) {
			s.Description = strings.TrimSpace(descriptions[i])
		}
		out = append(out, s)
	}
	return out
}

// serviceRowsForForm rebuilds the service-menu cards from a failed submit's
// form values (or an existing site's services, for the edit form), so
// nothing typed is lost on reload. Always returns at least one (possibly
// empty) row so the form has one to render.
func serviceRowsForForm(values url.Values) []domain.Service {
	labels := values["service_label"]
	prices := values["service_price"]
	descriptions := values["service_description"]
	n := len(labels)
	if len(prices) > n {
		n = len(prices)
	}
	if len(descriptions) > n {
		n = len(descriptions)
	}
	rows := make([]domain.Service, n)
	for i := range rows {
		if i < len(labels) {
			rows[i].Label = labels[i]
		}
		if i < len(prices) {
			rows[i].PriceText = prices[i]
		}
		if i < len(descriptions) {
			rows[i].Description = descriptions[i]
		}
	}
	if len(rows) == 0 {
		rows = append(rows, domain.Service{})
	}
	return rows
}

// serviceRowsForDisplay adapts a site's stored services to the repeatable
// service-menu card form, always returning at least one (possibly empty)
// row so the edit form has one to render.
func serviceRowsForDisplay(services []domain.Service) []domain.Service {
	if len(services) == 0 {
		return []domain.Service{{}}
	}
	return services
}

// parseFAQRows reads the repeatable FAQ cards — faq_question and faq_answer
// are each submitted as one value per row, in the same order, so the i-th
// value of each names one row. Rows with no question are dropped.
func parseFAQRows(r *http.Request) []domain.FAQItem {
	questions := r.Form["faq_question"]
	answers := r.Form["faq_answer"]
	var out []domain.FAQItem
	for i, question := range questions {
		question = strings.TrimSpace(question)
		if question == "" {
			continue
		}
		f := domain.FAQItem{Question: question, SortOrder: len(out)}
		if i < len(answers) {
			f.Answer = strings.TrimSpace(answers[i])
		}
		out = append(out, f)
	}
	return out
}

// faqRowsForDisplay adapts a site's stored FAQ items to the repeatable FAQ
// card form, always returning at least one (possibly empty) row so the edit
// form has one to render.
func faqRowsForDisplay(items []domain.FAQItem) []domain.FAQItem {
	if len(items) == 0 {
		return []domain.FAQItem{{}}
	}
	return items
}

// parseStaffRows reads the repeatable staff cards — staff_name, staff_role,
// staff_photo_url, and staff_bio are each submitted as one value per row, in
// the same order, so the i-th value of each names one row. Rows with no name
// are dropped.
func parseStaffRows(r *http.Request) []domain.StaffMember {
	names := r.Form["staff_name"]
	roles := r.Form["staff_role"]
	photoURLs := r.Form["staff_photo_url"]
	bios := r.Form["staff_bio"]
	var out []domain.StaffMember
	for i, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		m := domain.StaffMember{Name: name, SortOrder: len(out)}
		if i < len(roles) {
			m.Role = strings.TrimSpace(roles[i])
		}
		if i < len(photoURLs) {
			m.PhotoURL = strings.TrimSpace(photoURLs[i])
		}
		if i < len(bios) {
			m.Bio = strings.TrimSpace(bios[i])
		}
		out = append(out, m)
	}
	return out
}

// staffRowsForDisplay adapts a site's stored staff members to the repeatable
// staff card form, always returning at least one (possibly empty) row so the
// edit form has one to render.
func staffRowsForDisplay(members []domain.StaffMember) []domain.StaffMember {
	if len(members) == 0 {
		return []domain.StaffMember{{}}
	}
	return members
}

func parseCertifications(s string) []domain.Certification {
	var out []domain.Certification
	for i, label := range splitLines(s) {
		out = append(out, domain.Certification{Label: label, SortOrder: i})
	}
	return out
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

func parseServiceAreas(s string) []domain.ServiceArea {
	var out []domain.ServiceArea
	for i, area := range splitLines(s) {
		out = append(out, domain.ServiceArea{Area: area, SortOrder: i})
	}
	return out
}

func serviceAreasToLines(a []domain.ServiceArea) string {
	lines := make([]string, len(a))
	for i, x := range a {
		lines[i] = x.Area
	}
	return strings.Join(lines, "\n")
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

// parseTestimonialRows reads the wizard's repeatable testimonial cards —
// testimonial_name/testimonial_role/testimonial_quote are each submitted as
// one value per row, in the same order, so the i-th value of each names one
// row. Rows with no quote are dropped.
func parseTestimonialRows(r *http.Request) []domain.Testimonial {
	names := r.Form["testimonial_name"]
	roles := r.Form["testimonial_role"]
	quotes := r.Form["testimonial_quote"]
	var out []domain.Testimonial
	for i, quote := range quotes {
		quote = strings.TrimSpace(quote)
		if quote == "" {
			continue
		}
		t := domain.Testimonial{Quote: quote, SortOrder: len(out)}
		if i < len(names) {
			t.AuthorName = strings.TrimSpace(names[i])
		}
		if i < len(roles) {
			t.AuthorRole = strings.TrimSpace(roles[i])
		}
		out = append(out, t)
	}
	return out
}

// testimonialRowsForForm rebuilds the wizard's testimonial cards from a
// failed submit's form values, so nothing typed is lost on reload. Always
// returns at least one (possibly empty) row so the form has one to render.
func testimonialRowsForForm(values url.Values) []domain.Testimonial {
	names := values["testimonial_name"]
	roles := values["testimonial_role"]
	quotes := values["testimonial_quote"]
	n := len(quotes)
	if len(names) > n {
		n = len(names)
	}
	if len(roles) > n {
		n = len(roles)
	}
	rows := make([]domain.Testimonial, n)
	for i := range rows {
		if i < len(names) {
			rows[i].AuthorName = names[i]
		}
		if i < len(roles) {
			rows[i].AuthorRole = roles[i]
		}
		if i < len(quotes) {
			rows[i].Quote = quotes[i]
		}
	}
	if len(rows) == 0 {
		rows = append(rows, domain.Testimonial{})
	}
	return rows
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
