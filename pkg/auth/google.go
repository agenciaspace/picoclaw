package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleOAuthConfig holds the parameters needed for Google OAuth2.
type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
	Port         int // local callback port, default 1456
}

// DefaultGoogleScopes returns the default OAuth scopes for picoclaw Google integration.
func DefaultGoogleScopes() []string {
	return []string{
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/gmail.send",
		"https://www.googleapis.com/auth/calendar",
	}
}

func (g GoogleOAuthConfig) oauth2Config(redirectURL string) *oauth2.Config {
	scopes := g.Scopes
	if len(scopes) == 0 {
		scopes = DefaultGoogleScopes()
	}
	return &oauth2.Config{
		ClientID:     g.ClientID,
		ClientSecret: g.ClientSecret,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURL,
	}
}

func (g GoogleOAuthConfig) port() int {
	if g.Port > 0 {
		return g.Port
	}
	return 1456
}

// LoginGoogle performs the OAuth2 authorization code flow for Google.
// It starts a local HTTP server, opens the browser, and waits for the callback.
func LoginGoogle(cfg GoogleOAuthConfig) (*AuthCredential, error) {
	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("generating state: %w", err)
	}

	redirectURI := fmt.Sprintf("http://localhost:%d/auth/google/callback", cfg.port())
	oauthCfg := cfg.oauth2Config(redirectURI)

	authURL := oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	resultCh := make(chan googleCallbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/google/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			resultCh <- googleCallbackResult{err: fmt.Errorf("state mismatch")}
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			resultCh <- googleCallbackResult{err: fmt.Errorf("no code received: %s", errMsg)}
			http.Error(w, "No authorization code received", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><h2>Google authentication successful!</h2><p>You can close this window and return to picoclaw.</p></body></html>")
		resultCh <- googleCallbackResult{code: code}
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.port()))
	if err != nil {
		return nil, fmt.Errorf("starting callback server on port %d: %w", cfg.port(), err)
	}

	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	fmt.Printf("Open this URL to authenticate with Google:\n\n%s\n\n", authURL)
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Could not open browser automatically.\nPlease open this URL manually:\n\n%s\n\n", authURL)
	}
	fmt.Println("Waiting for Google authentication in browser...")

	select {
	case result := <-resultCh:
		if result.err != nil {
			return nil, result.err
		}
		return exchangeGoogleCode(oauthCfg, result.code)
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("Google authentication timed out after 5 minutes")
	}
}

type googleCallbackResult struct {
	code string
	err  error
}

func exchangeGoogleCode(cfg *oauth2.Config, code string) (*AuthCredential, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchanging Google auth code: %w", err)
	}

	var expiresAt time.Time
	if !token.Expiry.IsZero() {
		expiresAt = token.Expiry
	}

	cred := &AuthCredential{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    expiresAt,
		Provider:     "google",
		AuthMethod:   "oauth",
	}

	// Try to extract user email from ID token if present
	if idTokenRaw, ok := token.Extra("id_token").(string); ok && idTokenRaw != "" {
		if claims, err := parseJWTClaims(idTokenRaw); err == nil {
			if email, ok := claims["email"].(string); ok {
				cred.AccountID = email
			}
		}
	}

	return cred, nil
}

// RefreshGoogleToken refreshes an expired Google access token using the stored refresh token.
func RefreshGoogleToken(cred *AuthCredential, clientID, clientSecret string) (*AuthCredential, error) {
	if cred.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available for Google")
	}

	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
	}

	tokenSource := cfg.TokenSource(context.Background(), &oauth2.Token{
		RefreshToken: cred.RefreshToken,
	})

	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("refreshing Google token: %w", err)
	}

	refreshed := &AuthCredential{
		AccessToken:  newToken.AccessToken,
		RefreshToken: newToken.RefreshToken,
		ExpiresAt:    newToken.Expiry,
		Provider:     "google",
		AuthMethod:   "oauth",
		AccountID:    cred.AccountID,
	}

	// Keep the old refresh token if the new one is empty
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = cred.RefreshToken
	}

	return refreshed, nil
}

// GetGoogleToken returns a valid Google access token, refreshing if needed.
// It loads the credential from the auth store and auto-refreshes if expired.
func GetGoogleToken(clientID, clientSecret string) (string, error) {
	cred, err := GetCredential("google")
	if err != nil {
		return "", fmt.Errorf("loading Google credential: %w", err)
	}
	if cred == nil {
		return "", fmt.Errorf("not authenticated with Google. Run: picoclaw auth login --provider google")
	}

	// Refresh if needed
	if cred.NeedsRefresh() || cred.IsExpired() {
		refreshed, err := RefreshGoogleToken(cred, clientID, clientSecret)
		if err != nil {
			return "", fmt.Errorf("refreshing Google token: %w", err)
		}
		if err := SetCredential("google", refreshed); err != nil {
			return "", fmt.Errorf("saving refreshed Google token: %w", err)
		}
		return refreshed.AccessToken, nil
	}

	return cred.AccessToken, nil
}

// GoogleTokenJSON returns the stored Google token as a JSON-serialized oauth2.Token.
// This is useful for libraries that need the full token object.
func GoogleTokenJSON(clientID, clientSecret string) ([]byte, error) {
	cred, err := GetCredential("google")
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, fmt.Errorf("not authenticated with Google")
	}

	tok := &oauth2.Token{
		AccessToken:  cred.AccessToken,
		RefreshToken: cred.RefreshToken,
		Expiry:       cred.ExpiresAt,
		TokenType:    "Bearer",
	}

	return json.Marshal(tok)
}
