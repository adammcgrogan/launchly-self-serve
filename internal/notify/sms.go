// Package notify sends SMS lead alerts to business owners via Twilio. It is
// entirely optional: SMSClient.Configured reports false whenever Twilio
// credentials aren't set (the default), and callers skip sending rather
// than making a doomed API call — same "unset key = feature off" pattern as
// internal/email for Resend.
package notify

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SMSClient struct {
	accountSID string
	authToken  string
	from       string
	httpClient *http.Client
}

func NewSMSClient(accountSID, authToken, from string) *SMSClient {
	return &SMSClient{
		accountSID: accountSID,
		authToken:  authToken,
		from:       from,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Configured reports whether Twilio credentials are set.
func (c *SMSClient) Configured() bool {
	return c.accountSID != "" && c.authToken != "" && c.from != ""
}

// SendLeadAlert texts a business owner about a new website enquiry. The
// message is deliberately short with no visitor message body — for length
// and privacy, it points the owner at email/dashboard for details.
func (c *SMSClient) SendLeadAlert(to, businessName, visitorName string) error {
	body := fmt.Sprintf("Launchly: new enquiry from %s for %s. Check your email or dashboard to reply.", visitorName, businessName)
	form := url.Values{
		"From": {c.from},
		"To":   {to},
		"Body": {body},
	}
	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", c.accountSID)
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.accountSID, c.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("twilio api error: %s", resp.Status)
	}
	return nil
}
