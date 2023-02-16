package telemetry

import (
	"fmt"

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
	installId string
	sessionId string
}

func NewAppInsight(installId string, sessionid string) Client {
	// The instrument key of a MS managed application insights
	const instrumentKey = "1bfe1d29-b42e-49b5-9d51-77514f85b37b"
	return AppInsightClient{
		TelemetryClient: appinsights.NewTelemetryClient(instrumentKey),
		installId:       installId,
		sessionId:       sessionid,
	}
}

func (c AppInsightClient) Trace(level Level, msg string) {
	msg = fmt.Sprintf("[%s (%s)] %s", c.installId, c.sessionId, msg)
	c.TrackTrace(msg, contracts.SeverityLevel(level))
}

func (c AppInsightClient) Close() {
	<-c.Channel().Close()
}
