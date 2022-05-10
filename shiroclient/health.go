package shiroclient

import (
	"context"
	"encoding/json"
	"fmt"
)

type jsonFieldError struct {
	desc  string
	typ   string
	field string
}

func (err *jsonFieldError) Error() string {
	if err.typ == "" {
		return fmt.Sprintf("%s expected %s field", err.desc, err.field)
	}
	return fmt.Sprintf("%s expected %s %s field", err.desc, err.typ, err.field)
}

func stringFieldError(desc string, field string) *jsonFieldError {
	return &jsonFieldError{desc, "string", field}
}

type healthcheck []HealthCheckReport

func (c healthcheck) Reports() []HealthCheckReport {
	return c
}

type healthreport struct {
	timestamp      string
	status         string
	servicename    string
	serviceversion string
}

func (h *healthreport) Timestamp() string      { return h.timestamp }
func (h *healthreport) Status() string         { return h.status }
func (h *healthreport) ServiceName() string    { return h.servicename }
func (h *healthreport) ServiceVersion() string { return h.serviceversion }

var _ HealthCheckReport = (*healthreport)(nil)

// NOTE:  convertHealthReport doesn't unmarshal directly into the healthreport
// struct to maintain semantics similar to other json decoding happening in
// this package (e.g. semantics around handling incorrect letter cases and
// missing fields).
func unmarshalHealthResponse(r []byte) (healthcheck, error) {
	// NOTE: rawResp *does* use json struct deserialization to ease handling of
	// any exception object which may be passed from upstream.
	var rawResp struct {
		Reports   []interface{}
		Exception *json.RawMessage
	}
	err := json.Unmarshal(r, &rawResp)
	if err != nil {
		return nil, fmt.Errorf("invalid result format: %w", err)
	}
	if rawResp.Exception != nil {
		return nil, fmt.Errorf("remote exception: %s", *rawResp.Exception)
	}
	reports := make(healthcheck, len(rawResp.Reports))
	for i, rawReport := range rawResp.Reports {
		reports[i], err = convertHealthReport(rawReport)
		if err != nil {
			return nil, err
		}
	}
	return reports, nil
}

func convertHealthReport(rawReport interface{}) (*healthreport, error) {
	m, ok := rawReport.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("health check report: expected an object")
	}
	const errdesc = "health check report"
	ts, ok := m["timestamp"].(string)
	if !ok {
		return nil, stringFieldError(errdesc, "timestamp")
	}
	status, ok := m["status"].(string)
	if !ok {
		return nil, stringFieldError(errdesc, "status")
	}
	svc, ok := m["service_name"].(string)
	if !ok {
		return nil, stringFieldError(errdesc, "service_name")
	}
	ver, ok := m["service_version"].(string)
	if !ok {
		return nil, stringFieldError(errdesc, "service_version")
	}
	report := &healthreport{
		timestamp:      ts,
		status:         status,
		servicename:    svc,
		serviceversion: ver,
	}
	return report, nil
}

// smartHealthCheck is an internal interface that is not intended to be used in
// implementations outside of this package.  The interface is subject to
// change.
type smartHealthCheck interface {
	HealthCheck(ctx context.Context, services []string, configs ...Config) (HealthCheck, error)
}

// RemoteHealthCheck checks connectivity between the SDK client (e.g. oracle
// service) and upstream services including the phylum itself.  If the list of
// upstream services is empty the behavior of RemoteHealthCheck depends on
// client's implementation.  Clients created with NewMock do not support
// upstream service enumeration and will always invoke the mock phylum
// "healthcheck" endpoint.
//
// For clients that support RemoteHealthCheck service enumeration, like those
// created with NewRPC, services should be specified using canonical names
//		phylum
//		shiroclient_gateway
//		...
// Unrecognized service names are ignored, though may still be sent to upstream
// gateways.
//
// NOTE:  An RPC gateway must be a recent enough version to support
// specification of upstream services or it will otherwise fallback to invoking
// the phylum healthcheck endpoint.
func RemoteHealthCheck(ctx context.Context, client ShiroClient, services []string, configs ...Config) (HealthCheck, error) {

	switch client := client.(type) {
	case smartHealthCheck:
		return client.HealthCheck(ctx, services, configs...)
	default:
		resp, err := client.Call(ctx, "healthcheck", configs...)
		if err != nil {
			return nil, err
		}
		err = rpcError(resp)
		if err != nil {
			return nil, err
		}
		return unmarshalHealthResponse(resp.ResultJSON())
	}
}
