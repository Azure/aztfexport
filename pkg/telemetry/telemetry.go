package telemetry

import (
	"encoding/json"

	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"github.com/microsoft/ApplicationInsights-Go/appinsights/contracts"
)

type Level int

const (
	Verbose Level = iota
	Info
	Warn
	Error
	Critical
)

type Client interface {
	Trace(level Level, msg string)
	Close()
}

type NullClient struct{}

func NewNullClient() Client {
	return NullClient{}
}

func (NullClient) Trace(Level, string) {}
func (NullClient) Close()              {}

type AppInsightClient struct {
	appinsights.TelemetryClient
	subscriptionId string
	installId      string
	sessionId      string
}

func NewAppInsight(subscriptionId string, installId string, sessionid string) Client {
	// The instrument key of a MS managed application insights
	const instrumentKey = "1bfe1d29-b42e-49b5-9d51-77514f85b37b"
	return AppInsightClient{
		TelemetryClient: appinsights.NewTelemetryClient(instrumentKey),
		subscriptionId:  subscriptionId,
		installId:       installId,
		sessionId:       sessionid,
	}
}

type ApplicationInsightMessage struct {
	SubscriptionId string `json:"subscription_id"`
	InstallationId string `json:"installation_id"`
	SessionId      string `json:"session_id"`
	Payload        string `json:"payload"`
}

func (c AppInsightClient) Trace(level Level, payload string) {
	msg := ApplicationInsightMessage{
		SubscriptionId: c.subscriptionId,
		InstallationId: c.installId,
		SessionId:      c.sessionId,
		Payload:        payload,
	}
	b, _ := json.Marshal(msg)
	c.TrackTrace(string(b), contracts.SeverityLevel(level))
}

func (c AppInsightClient) Close() {
	<-c.Channel().Close()
}
