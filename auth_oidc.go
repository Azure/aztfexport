package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

var _ azcore.TokenCredential = &OidcCredential{}

type OidcCredential struct {
	requestToken string
	requestUrl   string

	token         string
	tokenFilePath string

	cred *azidentity.ClientAssertionCredential
}

type OidcCredentialOptions struct {
	azcore.ClientOptions
	TenantID      string
	ClientID      string
	RequestToken  string
	RequestUrl    string
	Token         string
	TokenFilePath string
}

func NewOidcCredential(options *OidcCredentialOptions) (*OidcCredential, error) {
	w := &OidcCredential{
		requestToken:  options.RequestToken,
		requestUrl:    options.RequestUrl,
		token:         options.Token,
		tokenFilePath: options.TokenFilePath,
	}

	cred, err := azidentity.NewClientAssertionCredential(options.TenantID, options.ClientID, w.getAssertion, &azidentity.ClientAssertionCredentialOptions{ClientOptions: options.ClientOptions})
	if err != nil {
		return nil, err
	}

	w.cred = cred
	return w, nil
}

func (w *OidcCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return w.cred.GetToken(ctx, opts)
}

func (w *OidcCredential) getAssertion(ctx context.Context) (string, error) {
	if w.token != "" {
		return w.token, nil
	}

	if w.tokenFilePath != "" {
		idTokenData, err := os.ReadFile(w.tokenFilePath)
		if err != nil {
			return "", fmt.Errorf("reading token file: %v", err)
		}

		return string(idTokenData), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.requestUrl, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("getAssertion: failed to build request")
	}

	query, err := url.ParseQuery(req.URL.RawQuery)
	if err != nil {
		return "", fmt.Errorf("getAssertion: cannot parse URL query")
	}

	if query.Get("audience") == "" {
		query.Set("audience", "api://AzureADTokenExchange")
		req.URL.RawQuery = query.Encode()
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", w.requestToken))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("getAssertion: cannot request token: %v", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("getAssertion: cannot parse response: %v", err)
	}

	if c := resp.StatusCode; c < 200 || c > 299 {
		return "", fmt.Errorf("getAssertion: received HTTP status %d with response: %s", resp.StatusCode, body)
	}

	var tokenRes struct {
		Count *int    `json:"count"`
		Value *string `json:"value"`
	}
	if err := json.Unmarshal(body, &tokenRes); err != nil {
		return "", fmt.Errorf("getAssertion: cannot unmarshal response: %v", err)
	}

	if tokenRes.Value == nil {
		return "", fmt.Errorf("getAssertion: nil JWT assertion received from OIDC provider")
	}

	return *tokenRes.Value, nil
}
