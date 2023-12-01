package rpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalHealthResponse_invalid(t *testing.T) {
	for i, test := range []struct {
		jsbody string
	}{
		{``},  // no response
		{`{`}, // matching right curly to avoid confusing editors: }
		{`{"exception": {"description": "BAD"}}`}, // presense of an exception
		{`{"reports": [{"timestamp": 1234}]}`},    // invalid field type
	} {
		resp, err := unmarshalHealthResponse([]byte(test.jsbody))
		assert.Error(t, err, "test %d success", i)
		assert.Nil(t, resp, "test %d response", i)
	}
}

func TestUnmarshalHealthResponse(t *testing.T) {
	for i, test := range []struct {
		jsbody  string
		reports []*healthreport
	}{
		{`{}`, []*healthreport{}},
		{`{"reports": []}`, []*healthreport{}},
		{`{"reports": [
			{
				"timestamp":"1234",
				"status": "UP",
				"service_name": "example",
				"service_version": "1.2.3"
			}
		]}`, []*healthreport{
			{
				timestamp:      "1234",
				status:         "UP",
				servicename:    "example",
				serviceversion: "1.2.3",
			},
		}},
		{`{"reports": [
			{
				"timestamp":"1234",
				"status": "UP",
				"service_name": "ex1",
				"service_version": "1.2.3"
			},
			{
				"timestamp":"1235",
				"status": "DOWN",
				"service_name": "ex2",
				"service_version": "2.3.4"
			}
		]}`, []*healthreport{
			{
				timestamp:      "1234",
				status:         "UP",
				servicename:    "ex1",
				serviceversion: "1.2.3",
			},
			{
				timestamp:      "1235",
				status:         "DOWN",
				servicename:    "ex2",
				serviceversion: "2.3.4",
			},
		}},
	} {
		resp, err := unmarshalHealthResponse([]byte(test.jsbody))
		assert.NoError(t, err, "test %d failure", i)
		reports := resp.Reports()
		assert.Equal(t, len(test.reports), len(reports), "test %d unexpected length", i)
		for j, r := range reports {
			assert.Equal(t, test.reports[j], r, "test %d report %d difference", i, j)
		}
	}
}

func TestGatewayHealthCheckURL(t *testing.T) {
	const endpoint = "https://localhost"
	list := func(name ...string) []string { return name }
	for i, test := range []struct {
		services []string
		url      string
	}{
		{list(), "https://localhost/health_check"},
		{list("phylum"), "https://localhost/health_check?service=phylum"},
		{list("fabric_peer", "phylum"), "https://localhost/health_check?service=fabric_peer&service=phylum"},
	} {
		u, err := gatewayHealthCheckURL(endpoint, test.services)
		if assert.NoError(t, err, "test %d", i) {
			assert.Equal(t, test.url, u, "test %d", i)
		}
	}
}
