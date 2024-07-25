package main

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log/slog"

	"github.com/Azure/aztfexport/pkg/config"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"software.sslmate.com/src/go-pkcs12"
)

type DefaultAzureCredentialOptions struct {
	AuthConfig               config.AuthConfig
	ClientOptions            azcore.ClientOptions
	DisableInstanceDiscovery bool
	// Only applies to certificate credential
	SendCertificateChain bool
}

// DefaultAzureCredential is a default credential chain for applications that will deploy to Azure.
// It attempts to authenticate with each of these credential types, in the following order, stopping
// when one provides a token:
//   - [ClientSecretCredential]
//   - [ClientCertificateCredential]
//   - [OIDCCredential]
//   - [ManagedIdentityCredential]
//   - [AzureCLICredential]
type DefaultAzureCredential struct {
	chain *azidentity.ChainedTokenCredential
}

// NewDefaultAzureCredential creates a DefaultAzureCredential. Pass nil for options to accept defaults.
func NewDefaultAzureCredential(logger slog.Logger, opt *DefaultAzureCredentialOptions) (*DefaultAzureCredential, error) {
	var creds []azcore.TokenCredential

	if opt == nil {
		opt = &DefaultAzureCredentialOptions{}
	}

	logger.Info("Building credential via client secret")
	if cred, err := azidentity.NewClientSecretCredential(
		opt.AuthConfig.TenantID,
		opt.AuthConfig.ClientID,
		opt.AuthConfig.ClientSecret,
		&azidentity.ClientSecretCredentialOptions{
			ClientOptions:              opt.ClientOptions,
			AdditionallyAllowedTenants: opt.AuthConfig.AuxiliaryTenantIDs,
			DisableInstanceDiscovery:   opt.DisableInstanceDiscovery,
		},
	); err == nil {
		logger.Info("Successfully built credential via client secret")
		creds = append(creds, cred)
	} else {
		logger.Warn("Building credential via client secret failed", "error", err)
	}

	logger.Info("Building credential via client certificaite")
	if cert, err := base64.StdEncoding.DecodeString(opt.AuthConfig.ClientCertificateEncoded); err != nil {
		logger.Warn("Building credential via client certificate failed", "error", fmt.Errorf("base64 decoidng certificate: %v", err))
	} else {
		// We are using a 3rd party module for parsing the certificate (the same one as is used by go-azure-sdk/sdk/auth/client_certificate_authorizer.go)
		// Reason can be found at: https://github.com/Azure/azure-sdk-for-go/issues/22906
		//certs, key, err := azidentity.ParseCertificates(cert, []byte(opt.AuthConfig.ClientCertificatePassword))
		key, cert, _, err := pkcs12.DecodeChain(cert, opt.AuthConfig.ClientCertificatePassword)
		if err == nil {
			if cred, err := azidentity.NewClientCertificateCredential(
				opt.AuthConfig.TenantID,
				opt.AuthConfig.ClientID,
				[]*x509.Certificate{cert},
				key,
				&azidentity.ClientCertificateCredentialOptions{
					ClientOptions:              opt.ClientOptions,
					AdditionallyAllowedTenants: opt.AuthConfig.AuxiliaryTenantIDs,
					DisableInstanceDiscovery:   opt.DisableInstanceDiscovery,
					SendCertificateChain:       opt.SendCertificateChain,
				},
			); err == nil {
				logger.Info("Successfully built credential via client certificate")
				creds = append(creds, cred)
			} else {
				logger.Warn("Building credential via client certificate failed", "error", err)
			}
		} else {
			logger.Warn("Building credential via client certificate failed", "error", fmt.Errorf(`failed to parse certificate: %v`, err))
		}
	}

	if !opt.AuthConfig.UseOIDC {
		logger.Info("OIDC credential skipped")
	} else {
		logger.Info("Building credential via OIDC")
		if cred, err := NewOidcCredential(&OidcCredentialOptions{
			ClientOptions: opt.ClientOptions,
			TenantID:      opt.AuthConfig.TenantID,
			ClientID:      opt.AuthConfig.ClientID,
			RequestToken:  opt.AuthConfig.OIDCTokenRequestToken,
			RequestUrl:    opt.AuthConfig.OIDCTokenRequestURL,
			Token:         opt.AuthConfig.OIDCAssertionToken,
		}); err == nil {
			logger.Info("Successfully built credential via OIDC")
			creds = append(creds, cred)
		} else {
			logger.Warn("Building credential via OIDC failed", "error", err)
		}
	}

	if !opt.AuthConfig.UseManagedIdentity {
		logger.Info("managed identity credential skipped")
	} else {
		logger.Info("Building credential via managed identity")
		if cred, err := azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
			ClientOptions: opt.ClientOptions,
			ID:            azidentity.ClientID(opt.AuthConfig.ClientID),
		}); err == nil {
			logger.Info("Successfully built credential via managed identity")
			creds = append(creds, cred)
		} else {
			logger.Warn("Building credential via managed identity failed", "error", err)
		}
	}

	if !opt.AuthConfig.UseAzureCLI {
		logger.Info("Azure CLI credential skipped")
	} else {
		logger.Info("Building credential via Azure CLI")
		if cred, err := azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{
			AdditionallyAllowedTenants: opt.AuthConfig.AuxiliaryTenantIDs,
			TenantID:                   opt.AuthConfig.TenantID,
		}); err == nil {
			logger.Info("Successfully built credential via Azure CLI")
			creds = append(creds, cred)
		} else {
			logger.Warn("Building credential via Azure CLI failed", "error", err)
		}
	}

	chain, err := azidentity.NewChainedTokenCredential(creds, nil)
	if err != nil {
		return nil, err
	}
	return &DefaultAzureCredential{chain: chain}, nil
}

// GetToken requests an access token from Azure Active Directory. This method is called automatically by Azure SDK clients.
func (c *DefaultAzureCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return c.chain.GetToken(ctx, opts)
}

var _ azcore.TokenCredential = (*DefaultAzureCredential)(nil)
