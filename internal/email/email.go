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

// wrap puts content inside the standard email shell.
func wrap(content string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#f8fafc;font-family:` + brandSansFont + `;">
  <table width="100%" cellpadding="0" cellspacing="0" style="background:#f8fafc;padding:40px 16px;">
    <tr><td align="center">
      <table width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;">
        <tr>
          <td style="background:` + brandIndigo + `;border-radius:12px 12px 0 0;padding:24px 32px;border-bottom:3px solid ` + brandAmber + `;">
            <span style="font-family:` + brandDisplayFont + `;color:#ffffff;font-size:21px;font-weight:700;letter-spacing:-0.3px;">Launchly</span>
          </td>
        </tr>
        <tr>
          <td style="background:#ffffff;padding:36px 32px;border-left:1px solid #e2e8f0;border-right:1px solid #e2e8f0;">
            ` + content + `
          </td>
        </tr>
        <tr>
          <td style="background:#f8fafc;border:1px solid #e2e8f0;border-top:none;border-radius:0 0 12px 12px;padding:20px 32px;text-align:center;">
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

func button(href, label, bg string) string {
	return fmt.Sprintf(`
<table width="100%%" cellpadding="0" cellspacing="0" style="margin:28px 0;">
  <tr><td align="center">
    <a href="%s" style="display:inline-block;background:%s;color:#ffffff;padding:14px 32px;border-radius:8px;text-decoration:none;font-weight:700;font-size:15px;font-family:%s;">%s</a>
  </td></tr>
</table>`, href, bg, brandSansFont, label)
}

func h1(text string) string {
	return fmt.Sprintf(`<h1 style="margin:0 0 16px;font-family:%s;font-size:23px;font-weight:700;color:#0f172a;line-height:1.3;">%s</h1>`, brandDisplayFont, text)
}

func p(text string) string {
	return fmt.Sprintf(`<p style="margin:0 0 16px;font-family:%s;font-size:15px;color:#334155;line-height:1.6;">%s</p>`, brandSansFont, text)
}

func divider() string {
	return `<hr style="border:none;border-top:1px solid #e2e8f0;margin:24px 0;">`
}

// SendWelcomeEmail is sent right after account signup, alongside (not
// instead of) Supabase's own verification email.
func (c *Client) SendWelcomeEmail(to, dashboardURL string) error {
	content := h1("Welcome to Launchly") +
		p("Your account is ready. Build your first site and it'll go live immediately, no waiting, no approval.") +
		button(dashboardURL, "Go to Your Dashboard", "#4F46E5") +
		divider() +
		p(`<span style="color:#64748b;font-size:13px;">Questions? Contact us at <a href="mailto:hello@launchly.ltd" style="color:#4F46E5;">hello@launchly.ltd</a></span>`)
	return c.Send(to, "Welcome to Launchly", wrap(content))
}

// SendLeadNotification forwards a contact-form submission to the business
// owner, with the visitor's email set as reply-to so they can respond directly.
func (c *Client) SendLeadNotification(to, businessName, visitorName, visitorEmail, phone, message string) error {
	rows := ""
	for _, f := range [][2]string{{"Name", visitorName}, {"Email", visitorEmail}, {"Phone", phone}} {
		if strings.TrimSpace(f[1]) == "" {
			continue
		}
		rows += fmt.Sprintf(`
<tr>
  <td style="padding:10px 14px;font-size:13px;font-weight:600;color:#64748b;white-space:nowrap;width:80px;">%s</td>
  <td style="padding:10px 14px;font-size:14px;color:#0f172a;">%s</td>
</tr>`, f[0], html.EscapeString(f[1]))
	}
	if strings.TrimSpace(message) != "" {
		rows += fmt.Sprintf(`
<tr>
  <td style="padding:10px 14px;font-size:13px;font-weight:600;color:#64748b;vertical-align:top;">Message</td>
  <td style="padding:10px 14px;font-size:14px;color:#0f172a;">%s</td>
</tr>`, html.EscapeString(message))
	}
	table := fmt.Sprintf(`
<table width="100%%" cellpadding="0" cellspacing="0" style="border:1px solid #e2e8f0;border-radius:8px;border-collapse:separate;border-spacing:0;overflow:hidden;margin:0 0 24px;">%s</table>`, rows)

	content := h1(fmt.Sprintf("New enquiry - %s", businessName)) +
		p(fmt.Sprintf("Someone just submitted an enquiry through your <strong>%s</strong> website:", businessName)) +
		table +
		divider() +
		p(`<span style="color:#64748b;font-size:13px;">This lead was submitted through your Launchly website contact form.</span>`)

	return c.sendWithReplyTo(to, fmt.Sprintf("New enquiry from your website - %s", businessName), wrap(content), visitorEmail)
}

func (c *Client) SendPaymentConfirmation(to, businessName string, plan domain.Plan) error {
	planLabel := "Starter"
	if plan == domain.PlanPro {
		planLabel = "Pro"
	}
	content := h1("Payment confirmed - you're all set!") +
		p(fmt.Sprintf("Thanks for subscribing to the <strong>%s plan</strong> for <strong>%s</strong>.", planLabel, businessName)) +
		p("Your site remains live. Enquiries from your site will keep landing straight in your inbox.") +
		divider() +
		p(`<span style="color:#64748b;font-size:13px;">Need to make changes? Log into your dashboard any time.</span>`)
	return c.Send(to, fmt.Sprintf("Payment confirmed for %s", businessName), wrap(content))
}

func (c *Client) SendCancellationConfirmation(to, businessName string) error {
	content := h1("Your subscription has been cancelled") +
		p(fmt.Sprintf("We've cancelled the subscription for <strong>%s</strong>.", businessName)) +
		p("If this was a mistake or you'd like to reactivate in future, just log back into your dashboard.") +
		divider() +
		p(`<span style="color:#64748b;font-size:13px;">We're sorry to see you go. Contact us at <a href="mailto:hello@launchly.ltd" style="color:#4F46E5;">hello@launchly.ltd</a></span>`)
	return c.Send(to, fmt.Sprintf("Subscription cancelled - %s", businessName), wrap(content))
}

func (c *Client) SendPaymentFailed(to, businessName string) error {
	content := h1("There was a problem with your payment") +
		p(fmt.Sprintf("We weren't able to collect your subscription payment for <strong>%s</strong>.", businessName)) +
		p("This can happen if a card has expired or has insufficient funds. Stripe will automatically retry the payment over the next few days.") +
		p("To avoid any disruption to your site, please update your payment details from your dashboard.") +
		divider() +
		p(`<span style="color:#64748b;font-size:13px;">Questions? Contact us at <a href="mailto:hello@launchly.ltd" style="color:#4F46E5;">hello@launchly.ltd</a></span>`)
	return c.Send(to, fmt.Sprintf("Action needed - payment failed for %s", businessName), wrap(content))
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
		button(dashboardURL, "Upgrade Now", "#4F46E5") +
		divider() +
		p(`<span style="color:#64748b;font-size:13px;">Questions? Contact us at <a href="mailto:hello@launchly.ltd" style="color:#4F46E5;">hello@launchly.ltd</a></span>`)
	subject := fmt.Sprintf("Your free trial ends in %s - %s", urgency, businessName)
	return c.Send(to, subject, wrap(content))
}

func (c *Client) SendAnalyticsDigest(to, businessName, frequency string, stats *domain.SiteStats, siteURL string) error {
	period, days := "weekly", "7 days"
	if frequency == "monthly" {
		period, days = "monthly", "30 days"
	}

	statsRow := fmt.Sprintf(`
<table width="100%%" cellpadding="0" cellspacing="0" style="margin:0 0 24px;">
  <tr>
    <td width="50%%" style="padding:0 8px 0 0;">
      <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:8px;padding:20px;text-align:center;">
        <div style="font-family:%s;font-size:34px;font-weight:700;color:%s;line-height:1;">%d</div>
        <div style="font-size:13px;color:#64748b;margin-top:4px;">Total visits</div>
      </div>
    </td>
    <td width="50%%" style="padding:0 0 0 8px;">
      <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:8px;padding:20px;text-align:center;">
        <div style="font-family:%s;font-size:34px;font-weight:700;color:%s;line-height:1;">%d</div>
        <div style="font-size:13px;color:#64748b;margin-top:4px;">Unique visitors</div>
      </div>
    </td>
  </tr>
</table>`, brandDisplayFont, brandIndigo, stats.TotalViews, brandDisplayFont, brandIndigo, stats.UniqueVisitors)

	var daysTable string
	if len(stats.ViewsByDay) > 0 {
		rows := ""
		for _, d := range stats.ViewsByDay {
			rows += fmt.Sprintf(`<tr>
  <td style="padding:7px 14px;font-size:13px;color:#334155;border-bottom:1px solid #f1f5f9;">%s</td>
  <td style="padding:7px 14px;font-size:13px;font-weight:700;color:#0f172a;border-bottom:1px solid #f1f5f9;text-align:right;">%d</td>
</tr>`, d.Day.Format("Mon 2 Jan"), d.Count)
		}
		daysTable = fmt.Sprintf(`<p style="margin:0 0 8px;font-size:12px;font-weight:700;color:#94a3b8;text-transform:uppercase;letter-spacing:.07em;">Views by day</p>
<table width="100%%" cellpadding="0" cellspacing="0" style="border:1px solid #e2e8f0;border-radius:8px;overflow:hidden;margin:0 0 24px;border-collapse:separate;border-spacing:0;">%s</table>`, rows)
	}

	var refTable string
	if len(stats.TopReferrers) > 0 {
		rows := ""
		for _, ref := range stats.TopReferrers {
			label := ref.Referrer
			if label == "" {
				label = "Direct / unknown"
			}
			rows += fmt.Sprintf(`<tr>
  <td style="padding:7px 14px;font-size:13px;color:#334155;border-bottom:1px solid #f1f5f9;">%s</td>
  <td style="padding:7px 14px;font-size:13px;font-weight:700;color:#0f172a;border-bottom:1px solid #f1f5f9;text-align:right;">%d</td>
</tr>`, html.EscapeString(label), ref.Count)
		}
		refTable = fmt.Sprintf(`<p style="margin:0 0 8px;font-size:12px;font-weight:700;color:#94a3b8;text-transform:uppercase;letter-spacing:.07em;">Where visitors came from</p>
<table width="100%%" cellpadding="0" cellspacing="0" style="border:1px solid #e2e8f0;border-radius:8px;overflow:hidden;margin:0 0 24px;border-collapse:separate;border-spacing:0;">%s</table>`, rows)
	}

	noDataNote := ""
	if stats.TotalViews == 0 {
		noDataNote = p(`<span style="color:#64748b;">No visits were recorded in this period. Once your site gets traffic, you'll see a full breakdown here.</span>`)
	}

	content := h1(fmt.Sprintf("Your %s website report", period)) +
		p(fmt.Sprintf("Here's how <strong>%s</strong> performed over the last %s.", businessName, days)) +
		statsRow + noDataNote + daysTable + refTable +
		button(siteURL, "View Your Website", "#4F46E5") +
		divider() +
		p(`<span style="color:#64748b;font-size:13px;">You're receiving this report because analytics is enabled for your site. Change the frequency any time from your dashboard.</span>`)

	subject := fmt.Sprintf("Your weekly website report - %s", businessName)
	if frequency == "monthly" {
		subject = fmt.Sprintf("Your monthly website report - %s", businessName)
	}
	return c.Send(to, subject, wrap(content))
}

// SendAdminAlert notifies the superadmin of noteworthy account events
// (cancellations, payment failures) — informational only, never blocking.
func (c *Client) SendAdminAlert(to, subject, message string) error {
	content := h1(subject) + p(message)
	return c.Send(to, subject, wrap(content))
}
