package config

type AuthConfig struct {
	Environment        string
	TenantID           string
	AuxiliaryTenantIDs []string

	ClientID                  string
	ClientSecret              string
	ClientCertificateEncoded  string
	ClientCertificatePassword string

	OIDCTokenRequestToken string
	OIDCTokenRequestURL   string
	OIDCAssertionToken    string

	UseAzureCLI        bool
	UseManagedIdentity bool
	UseOIDC            bool
}
