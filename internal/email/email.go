// Package email sends transactional app emails via Resend. Account-level
// emails (signup confirmation, password reset) are sent by Supabase Auth
// itself — this package only covers emails the app's own business logic
// triggers: welcome, lead notifications, billing, trial reminders, and
// analytics digests.
package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

type Client struct {
	apiKey     string
	from       string
	httpClient *http.Client
}

func New(apiKey, from string) *Client {
	return &Client{
		apiKey:     apiKey,
		from:       from,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type sendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	ReplyTo []string `json:"reply_to,omitempty"`
}

func (c *Client) Send(to, subject, html string) error {
	return c.sendWithReplyTo(to, subject, html, "")
}

func (c *Client) sendWithReplyTo(to, subject, html, replyTo string) error {
	payload := sendRequest{From: c.from, To: []string{to}, Subject: subject, HTML: html}
	if replyTo != "" {
		payload.ReplyTo = []string{replyTo}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("resend api error: %s", resp.Status)
	}
	return nil
}

// Brand tokens shared with web/templates/{public,dashboard}/base.html.
// Email clients can't load Google Fonts, so Fraunces/Inter fall back to
// safe display/system-sans stacks rather than @font-face.
const (
	brandDisplayFont = "Georgia,'Times New Roman',Times,serif"
	brandSansFont    = "-apple-system,BlinkMacSystemFont,'Segoe UI',Inter,Roboto,Helvetica,Arial,sans-serif"
	brandIndigo      = "#4F46E5"
	brandAmber       = "#F59E0B"
)

// wrap puts content inside the standard email shell: a thin amber accent
// bar, a white header with the wordmark (mirroring the site's actual white
// nav rather than a solid color block), an eyebrow label naming the kind of
// email this is, then the content and a quiet footer.
func wrap(eyebrowLabel, content string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#eef0f4;font-family:` + brandSansFont + `;">
  <table width="100%" cellpadding="0" cellspacing="0" style="background:#eef0f4;padding:40px 16px;">
    <tr><td align="center">
      <table width="100%" cellpadding="0" cellspacing="0" style="max-width:480px;background:#ffffff;border:1px solid #e2e8f0;border-radius:14px;overflow:hidden;">
        <tr><td style="background:` + brandAmber + `;height:4px;line-height:4px;font-size:0;">&nbsp;</td></tr>
        <tr>
          <td style="padding:22px 28px 18px;border-bottom:1px solid #eef1f6;">
            <span style="display:inline-block;width:9px;height:9px;border-radius:50%;background:` + brandAmber + `;margin-right:8px;"></span><span style="font-family:` + brandDisplayFont + `;font-size:19px;font-weight:700;color:#0f172a;letter-spacing:-0.2px;">Launchly</span>
          </td>
        </tr>
        <tr>
          <td style="padding:30px 28px 8px;">
            <span style="text-transform:uppercase;letter-spacing:.1em;font-size:11px;font-weight:700;color:#b45309;background:#fef3c7;display:inline-block;padding:4px 10px;border-radius:999px;">` + eyebrowLabel + `</span>
            <div style="margin-top:14px;">` + content + `</div>
          </td>
        </tr>
        <tr>
          <td style="padding:18px 28px 24px;text-align:center;border-top:1px solid #eef1f6;">
            <p style="margin:0 0 6px;font-family:` + brandDisplayFont + `;font-size:13px;font-weight:700;color:#cbd5e1;">Launchly</p>
            <p style="margin:0;font-family:` + brandSansFont + `;color:#94a3b8;font-size:12px;">
              Sent by <a href="https://launchly.ltd" style="color:` + brandIndigo + `;text-decoration:none;font-weight:600;">Launchly</a>
              &nbsp;·&nbsp; <a href="mailto:hello@launchly.ltd" style="color:#94a3b8;text-decoration:none;">hello@launchly.ltd</a>
            </p>
          </td>
        </tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`
}

// button renders the app's pill-shaped primary CTA — matches the
// rounded-full "Start free"/"Upgrade" buttons on the actual site rather
// than the 8px-radius rectangle the old template used.
func button(href, label string) string {
	return fmt.Sprintf(`
<table width="100%%" cellpadding="0" cellspacing="0" style="margin:26px 0 22px;">
  <tr><td align="center">
    <a href="%s" style="display:inline-block;background:%s;color:#ffffff;padding:13px 30px;border-radius:999px;text-decoration:none;font-weight:700;font-size:14px;font-family:%s;">%s</a>
  </td></tr>
</table>`, href, brandIndigo, brandSansFont, label)
}

func h1(text string) string {
	return fmt.Sprintf(`<h1 style="margin:0 0 14px;font-family:%s;font-size:22px;font-weight:700;color:#0f172a;line-height:1.32;">%s</h1>`, brandDisplayFont, text)
}

func p(text string) string {
	return fmt.Sprintf(`<p style="margin:0 0 20px;font-family:%s;font-size:14.5px;color:#334155;line-height:1.65;">%s</p>`, brandSansFont, text)
}

func divider() string {
	return `<hr style="border:none;border-top:1px solid #eef1f6;margin:8px 0 20px;">`
}

// infoCard wraps a set of infoRow/statRow rows in a bordered card with an
// indigo left rail — the "receipt" treatment used for the lead
// notification's contact fields and the analytics digest's breakdown lists.
func infoCard(rows string) string {
	return fmt.Sprintf(`<div style="border:1px solid #e2e8f0;border-left:3px solid %s;border-radius:10px;margin:0 0 22px;overflow:hidden;">%s</div>`, brandIndigo, rows)
}

// infoRow is a label/value row for infoCard, e.g. "Email  ggg@gmail.com".
func infoRow(label, value string, first bool) string {
	borderTop := "border-top:1px solid #eef1f6;"
	if first {
		borderTop = ""
	}
	return fmt.Sprintf(`
<table width="100%%" cellpadding="0" cellspacing="0" style="%s">
  <tr>
    <td style="padding:13px 18px;width:64px;text-transform:uppercase;letter-spacing:.06em;font-size:11px;font-weight:700;color:#94a3b8;vertical-align:top;">%s</td>
    <td style="padding:13px 18px;font-family:%s;font-size:14.5px;color:#0f172a;">%s</td>
  </tr>
</table>`, borderTop, label, brandSansFont, value)
}

// statRow is a label/count row for infoCard, e.g. "Mon 3 Jul  42".
func statRow(label string, count int, first bool) string {
	borderTop := "border-top:1px solid #eef1f6;"
	if first {
		borderTop = ""
	}
	return fmt.Sprintf(`
<table width="100%%" cellpadding="0" cellspacing="0" style="%s">
  <tr>
    <td style="padding:10px 18px;font-family:%s;font-size:13.5px;color:#334155;">%s</td>
    <td style="padding:10px 18px;font-family:%s;font-size:13.5px;font-weight:700;color:#0f172a;text-align:right;">%d</td>
  </tr>
</table>`, borderTop, brandSansFont, label, brandSansFont, count)
}

func sectionLabel(text string) string {
	return fmt.Sprintf(`<p style="margin:0 0 8px;font-family:%s;font-size:11px;font-weight:700;color:#94a3b8;text-transform:uppercase;letter-spacing:.08em;">%s</p>`, brandSansFont, text)
}

// statTile is one number+label tile in the analytics digest's stat grid,
// set in the display face and brand indigo to tie it back to the identity.
func statTile(value, label string) string {
	return fmt.Sprintf(`
<td width="50%%" style="padding:0 6px;">
  <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:10px;padding:18px;text-align:center;">
    <div style="font-family:%s;font-size:28px;font-weight:700;color:%s;line-height:1;">%s</div>
    <div style="font-size:12px;color:#64748b;margin-top:5px;">%s</div>
  </div>
</td>`, brandDisplayFont, brandIndigo, value, label)
}

// SendWelcomeEmail is sent right after account signup, alongside (not
// instead of) Supabase's own verification email.
func (c *Client) SendWelcomeEmail(to, dashboardURL string) error {
	content := h1("Your site is ready to build") +
		p("Your account is live. Build your first site and it goes live immediately — no waiting, no approval.") +
		button(dashboardURL, "Go to your dashboard") +
		divider() +
		p(`<span style="color:#94a3b8;font-size:13px;">Questions? Contact us at <a href="mailto:hello@launchly.ltd" style="color:#4F46E5;">hello@launchly.ltd</a></span>`)
	return c.Send(to, "Welcome to Launchly", wrap("Welcome", content))
}

// SendLeadNotification forwards a contact-form submission to the business
// owner, with the visitor's email set as reply-to so they can respond directly.
func (c *Client) SendLeadNotification(to, businessName, visitorName, visitorEmail, phone, message, serviceLabel, preferredTime string) error {
	rows := ""
	first := true
	for _, f := range [][2]string{{"Name", visitorName}, {"Email", visitorEmail}, {"Phone", phone}, {"Service", serviceLabel}, {"Preferred time", preferredTime}} {
		if strings.TrimSpace(f[1]) == "" {
			continue
		}
		value := html.EscapeString(f[1])
		if f[0] == "Email" {
			value = fmt.Sprintf(`<a href="mailto:%s" style="color:%s;text-decoration:none;">%s</a>`, value, brandIndigo, value)
		}
		rows += infoRow(f[0], value, first)
		first = false
	}
	if strings.TrimSpace(message) != "" {
		rows += infoRow("Message", html.EscapeString(message), first)
	}

	content := h1(fmt.Sprintf("Someone contacted %s", businessName)) +
		p("A visitor just submitted your contact form:") +
		infoCard(rows) +
		divider() +
		p(`<span style="color:#94a3b8;font-size:13px;">Reply to this email to respond directly — it goes straight to the visitor.</span>`)

	return c.sendWithReplyTo(to, fmt.Sprintf("New enquiry from your website - %s", businessName), wrap("New enquiry", content), visitorEmail)
}

func (c *Client) SendPaymentConfirmation(to, businessName string, plan domain.Plan) error {
	planLabel := "Starter"
	if plan == domain.PlanPro {
		planLabel = "Pro"
	}
	content := h1("You're all set") +
		p(fmt.Sprintf("Thanks for subscribing to the <strong>%s plan</strong> for <strong>%s</strong>.", planLabel, businessName)) +
		p("Your site remains live. Enquiries from your site will keep landing straight in your inbox.") +
		divider() +
		p(`<span style="color:#94a3b8;font-size:13px;">Need to make changes? Log into your dashboard any time.</span>`)
	return c.Send(to, fmt.Sprintf("Payment confirmed for %s", businessName), wrap("Payment confirmed", content))
}

func (c *Client) SendCancellationConfirmation(to, businessName string) error {
	content := h1("Your subscription has been cancelled") +
		p(fmt.Sprintf("We've cancelled the subscription for <strong>%s</strong>.", businessName)) +
		p("If this was a mistake or you'd like to reactivate in future, just log back into your dashboard.") +
		divider() +
		p(`<span style="color:#94a3b8;font-size:13px;">We're sorry to see you go. Contact us at <a href="mailto:hello@launchly.ltd" style="color:#4F46E5;">hello@launchly.ltd</a></span>`)
	return c.Send(to, fmt.Sprintf("Subscription cancelled - %s", businessName), wrap("Subscription cancelled", content))
}

func (c *Client) SendPaymentFailed(to, businessName string) error {
	content := h1("There was a problem with your payment") +
		p(fmt.Sprintf("We weren't able to collect your subscription payment for <strong>%s</strong>.", businessName)) +
		p("This can happen if a card has expired or has insufficient funds. Stripe will automatically retry the payment over the next few days.") +
		p("To avoid any disruption to your site, please update your payment details from your dashboard.") +
		divider() +
		p(`<span style="color:#94a3b8;font-size:13px;">Questions? Contact us at <a href="mailto:hello@launchly.ltd" style="color:#4F46E5;">hello@launchly.ltd</a></span>`)
	return c.Send(to, fmt.Sprintf("Action needed - payment failed for %s", businessName), wrap("Action needed", content))
}

// SendTrialWarning links straight to the dashboard upgrade button — there is
// no admin-sent payment link in the self-serve flow.
func (c *Client) SendTrialWarning(to, businessName, dashboardURL string, daysLeft int) error {
	urgency := fmt.Sprintf("%d days", daysLeft)
	if daysLeft == 1 {
		urgency = "1 day"
	}
	content := h1(fmt.Sprintf("Your free trial ends in %s", urgency)) +
		p(fmt.Sprintf("Your <strong>%s</strong> website's 14-day free trial ends in <strong>%s</strong>.", businessName, urgency)) +
		p("To keep your site online, upgrade from your dashboard. It only takes a minute.") +
		button(dashboardURL, "Upgrade now") +
		divider() +
		p(`<span style="color:#94a3b8;font-size:13px;">Questions? Contact us at <a href="mailto:hello@launchly.ltd" style="color:#4F46E5;">hello@launchly.ltd</a></span>`)
	subject := fmt.Sprintf("Your free trial ends in %s - %s", urgency, businessName)
	return c.Send(to, subject, wrap("Free trial", content))
}

// SendSitePaused notifies an owner their trial ended and the site is now
// paused. Reactivation links straight to the dashboard upgrade button — the
// same self-serve checkout used everywhere else.
func (c *Client) SendSitePaused(to, businessName, dashboardURL string) error {
	content := h1("Your site has been paused") +
		p(fmt.Sprintf("The free trial for <strong>%s</strong> has ended, so it's paused and no longer visible to visitors.", businessName)) +
		p("Reactivating takes one click — upgrade from your dashboard any time and your site comes straight back online.") +
		button(dashboardURL, "Reactivate my site") +
		divider() +
		p(`<span style="color:#94a3b8;font-size:13px;">Questions? Contact us at <a href="mailto:hello@launchly.ltd" style="color:#4F46E5;">hello@launchly.ltd</a></span>`)
	return c.Send(to, fmt.Sprintf("Your site is paused - %s", businessName), wrap("Site paused", content))
}

func (c *Client) SendAnalyticsDigest(to, businessName, frequency string, stats *domain.SiteStats, siteURL string) error {
	period, days := "weekly", "7 days"
	if frequency == "monthly" {
		period, days = "monthly", "30 days"
	}

	statsRow := fmt.Sprintf(`<table width="100%%" cellpadding="0" cellspacing="0" style="margin:0 0 22px;"><tr>%s%s</tr></table>`,
		statTile(fmt.Sprintf("%d", stats.TotalViews), "Total visits"),
		statTile(fmt.Sprintf("%d", stats.UniqueVisitors), "Unique visitors"))

	var daysTable string
	if len(stats.ViewsByDay) > 0 {
		rows := ""
		for i, d := range stats.ViewsByDay {
			rows += statRow(d.Day.Format("Mon 2 Jan"), d.Count, i == 0)
		}
		daysTable = sectionLabel("Views by day") + infoCard(rows)
	}

	var refTable string
	if len(stats.TopReferrers) > 0 {
		rows := ""
		for i, ref := range stats.TopReferrers {
			label := ref.Referrer
			if label == "" {
				label = "Direct / unknown"
			}
			rows += statRow(html.EscapeString(label), ref.Count, i == 0)
		}
		refTable = sectionLabel("Where visitors came from") + infoCard(rows)
	}

	noDataNote := ""
	if stats.TotalViews == 0 {
		noDataNote = p(`<span style="color:#94a3b8;">No visits were recorded in this period. Once your site gets traffic, you'll see a full breakdown here.</span>`)
	}

	content := h1(fmt.Sprintf("Your %s website report", period)) +
		p(fmt.Sprintf("Here's how <strong>%s</strong> performed over the last %s.", businessName, days)) +
		statsRow + noDataNote + daysTable + refTable +
		button(siteURL, "View your website") +
		divider() +
		p(`<span style="color:#94a3b8;font-size:13px;">You're receiving this report because analytics is enabled for your site. Change the frequency any time from your dashboard.</span>`)

	subject := fmt.Sprintf("Your weekly website report - %s", businessName)
	eyebrowLabel := "Weekly report"
	if frequency == "monthly" {
		subject = fmt.Sprintf("Your monthly website report - %s", businessName)
		eyebrowLabel = "Monthly report"
	}
	return c.Send(to, subject, wrap(eyebrowLabel, content))
}

// SendAdminAlert notifies the superadmin of noteworthy account events
// (cancellations, payment failures) — informational only, never blocking.
func (c *Client) SendAdminAlert(to, subject, message string) error {
	content := h1(subject) + p(message)
	return c.Send(to, subject, wrap("Alert", content))
}
