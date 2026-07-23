package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ErrRedirectURINotAllowed guards the redirect_uri a browser may hand us.
// Without the allowlist an attacker could point the exchange at a URI they
// control and harvest codes issued for this client.
var ErrRedirectURINotAllowed = errors.New("redirect_uri is not allowed")

type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
}

type GoogleProvider struct {
	config *oauth2.Config
	// allowedRedirects are the exact redirect_uri values a client may ask the
	// exchange to use, mirroring what is registered in Google Cloud Console.
	allowedRedirects map[string]struct{}
}

func NewGoogleProvider(clientID, clientSecret, redirectURL string, allowedRedirects []string) *GoogleProvider {
	allowed := make(map[string]struct{}, len(allowedRedirects))
	for _, uri := range allowedRedirects {
		if trimmed := strings.TrimRight(strings.TrimSpace(uri), "/"); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}

	return &GoogleProvider{
		allowedRedirects: allowed,
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes: []string{
				"https://www.googleapis.com/auth/userinfo.email",
				"https://www.googleapis.com/auth/userinfo.profile",
			},
			Endpoint: google.Endpoint,
		},
	}
}

func (p *GoogleProvider) GetAuthURL(state string) string {
	return p.config.AuthCodeURL(state)
}

// ExchangeCode trades an authorization code for the user's profile.
//
// redirectURI must be the exact value the browser sent to Google, because the
// token endpoint rejects a mismatch. Empty means the popup flow, which uses the
// configured default ("postmessage").
func (p *GoogleProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (userInfo *GoogleUserInfo, err error) {
	cfg, err := p.configFor(redirectURI)
	if err != nil {
		return nil, err
	}

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	client := cfg.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close response body: %w", closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to fetch user info from google")
	}

	var info GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &info, nil
}

// configFor returns the oauth2 config to exchange with, swapping in a
// caller-supplied redirect_uri only when it is on the allowlist.
func (p *GoogleProvider) configFor(redirectURI string) (*oauth2.Config, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(redirectURI), "/")
	if trimmed == "" || trimmed == p.config.RedirectURL {
		return p.config, nil
	}

	if _, ok := p.allowedRedirects[trimmed]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrRedirectURINotAllowed, trimmed)
	}

	clone := *p.config
	clone.RedirectURL = trimmed
	return &clone, nil
}
