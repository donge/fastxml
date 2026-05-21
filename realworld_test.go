package fastxml

// Real-world XML samples adapted from probe traffic tests in
// github.com/servicewall/whisky/pkg/util/xml2json_test.go
//
// Original tests called Xml2Json([]byte) which wraps Parse+MarshalTo.
// Here we exercise the same inputs directly against the fastxml API.

import (
	"encoding/json"
	"testing"
)

// helper: parse XML, marshal to JSON, unmarshal JSON into map.
func parseToMap(t *testing.T, xmlInput []byte) map[string]any {
	t.Helper()
	var p Parser
	v, err := p.ParseBytes(xmlInput)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	out := v.MarshalTo(nil)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	return m
}

// TestRealWorld_S3Error: AWS S3 403 error response (application/xml, no compression).
// Captured: HTTP/1.1 403 Forbidden, Server: AmazonS3, Content-Length: 231
func TestRealWorld_S3Error(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>AccessDenied</Code><Message>Access Denied</Message><RequestId>109FD74E22074065</RequestId><HostId>DpoVJmoWN1mhMGJWOx7tvXTl2nFy+j30nc1SU8bSKcB3FKsGea+njm/ZPAGp67G3</HostId></Error>`)

	m := parseToMap(t, input)

	errObj, ok := m["Error"].(map[string]any)
	if !ok {
		t.Fatalf("expected root key 'Error', got keys: %v", mapKeys(m))
	}

	want := map[string]string{
		"Code":      "AccessDenied",
		"Message":   "Access Denied",
		"RequestId": "109FD74E22074065",
	}
	for field, wantVal := range want {
		gotVal, ok := errObj[field].(string)
		if !ok {
			t.Errorf("Error.%s: expected string %q, got %T %v", field, wantVal, errObj[field], errObj[field])
			continue
		}
		if gotVal != wantVal {
			t.Errorf("Error.%s = %q, want %q", field, gotVal, wantVal)
		}
	}
}

// TestRealWorld_AdobeAIRUpdate: Adobe AIR software update descriptor (application/xml).
// Captured: HTTP/1.1 200 OK, Server: AmazonS3, Content-Length: 407
// Tests: XML namespace attribute on root, nested text-only elements, empty element.
func TestRealWorld_AdobeAIRUpdate(t *testing.T) {
	input := []byte(`<?xml version="1.0" encoding="utf-8"?>
<update xmlns="http://ns.adobe.com/air/framework/update/description/1.0">
  <required>true</required>
  <version>1.20130116182826</version>
  <major_version>1.20120702164818</major_version>
  <required_version>1.20120702164818</required_version>
  <id>com.hipchat</id>
  <url>http://downloads.hipchat.com/hipchat.air</url>
  <description></description>
</update>`)

	m := parseToMap(t, input)

	update, ok := m["update"].(map[string]any)
	if !ok {
		t.Fatalf("expected root key 'update', got keys: %v", mapKeys(m))
	}

	wantFields := map[string]string{
		"required":         "true",
		"version":          "1.20130116182826",
		"major_version":    "1.20120702164818",
		"required_version": "1.20120702164818",
		"id":               "com.hipchat",
		"url":              "http://downloads.hipchat.com/hipchat.air",
	}
	for field, wantVal := range wantFields {
		gotVal, ok := update[field].(string)
		if !ok {
			t.Errorf("update.%s: expected string %q, got %T %v", field, wantVal, update[field], update[field])
			continue
		}
		if gotVal != wantVal {
			t.Errorf("update.%s = %q, want %q", field, gotVal, wantVal)
		}
	}
}

// TestRealWorld_CrossDomainPolicy: Flash cross-domain policy (application/xml).
// Captured: HTTP/1.1 200 OK, Content-Length: 178
// Tests: element attributes rendered as "@attr" keys, multiple child elements,
// site-control with a single attribute.
func TestRealWorld_CrossDomainPolicy(t *testing.T) {
	input := []byte(`<?xml version="1.0" ?>
<cross-domain-policy>
<site-control permitted-cross-domain-policies="master-only"/>
<allow-access-from domain="*" secure="false"/>
</cross-domain-policy>`)

	m := parseToMap(t, input)

	cdp, ok := m["cross-domain-policy"].(map[string]any)
	if !ok {
		t.Fatalf("expected root key 'cross-domain-policy', got keys: %v", mapKeys(m))
	}

	sc, ok := cdp["site-control"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'site-control' object, got %T %v", cdp["site-control"], cdp["site-control"])
	}
	if got, _ := sc["@permitted-cross-domain-policies"].(string); got != "master-only" {
		t.Errorf("site-control[@permitted-cross-domain-policies] = %q, want %q", got, "master-only")
	}

	aaf, ok := cdp["allow-access-from"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'allow-access-from' object, got %T %v", cdp["allow-access-from"], cdp["allow-access-from"])
	}
	if got, _ := aaf["@domain"].(string); got != "*" {
		t.Errorf("allow-access-from[@domain] = %q, want %q", got, "*")
	}
	if got, _ := aaf["@secure"].(string); got != "false" {
		t.Errorf("allow-access-from[@secure] = %q, want %q", got, "false")
	}
}

func mapKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
