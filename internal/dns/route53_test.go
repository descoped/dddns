package dns

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fixedNow returns a deterministic time so SigV4 signatures are stable under test.
func fixedNow() time.Time { return time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC) }

func newTestClient(t *testing.T, handler http.HandlerFunc) *Route53Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Route53Client{
		accessKey:    "AKIDEXAMPLE",
		secretKey:    "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		hostedZoneID: "Z123456",
		hostname:     "test.example.com",
		ttl:          300,
		httpClient:   srv.Client(),
		baseURL:      srv.URL,
		now:          fixedNow,
	}
}

const sampleListResponse = `<?xml version="1.0" encoding="UTF-8"?>
<ListResourceRecordSetsResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ResourceRecordSets>
    <ResourceRecordSet>
      <Name>test.example.com.</Name>
      <Type>A</Type>
      <TTL>300</TTL>
      <ResourceRecords>
        <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
      </ResourceRecords>
    </ResourceRecordSet>
  </ResourceRecordSets>
  <IsTruncated>false</IsTruncated>
  <MaxItems>1</MaxItems>
</ListResourceRecordSetsResponse>`

const sampleEmptyListResponse = `<?xml version="1.0" encoding="UTF-8"?>
<ListResourceRecordSetsResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ResourceRecordSets></ResourceRecordSets>
  <IsTruncated>false</IsTruncated>
  <MaxItems>1</MaxItems>
</ListResourceRecordSetsResponse>`

const sampleChangeResponse = `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeInfo>
    <Id>/change/C2682N5HXP0BZ4</Id>
    <Status>PENDING</Status>
    <SubmittedAt>2026-04-17T12:00:00Z</SubmittedAt>
  </ChangeInfo>
</ChangeResourceRecordSetsResponse>`

const sampleErrorResponse = `<?xml version="1.0" encoding="UTF-8"?>
<ErrorResponse>
  <Error>
    <Type>Sender</Type>
    <Code>InvalidChangeBatch</Code>
    <Message>The request contained an invalid value.</Message>
  </Error>
  <RequestId>abc-123</RequestId>
</ErrorResponse>`

func TestRoute53Client_GetCurrentIP(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/hostedzone/Z123456/rrset") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("type") != "A" {
			t.Errorf("expected type=A, got %q", r.URL.Query().Get("type"))
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("Authorization header missing (SigV4 signing failed)")
		}
		if r.Header.Get("X-Amz-Date") == "" {
			t.Error("X-Amz-Date header missing")
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, sampleListResponse)
	})

	ip, err := client.GetCurrentIP(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentIP failed: %v", err)
	}
	if ip != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %s", ip)
	}
}

func TestRoute53Client_GetCurrentIP_NotFound(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, sampleEmptyListResponse)
	})

	_, err := client.GetCurrentIP(context.Background())
	if err == nil {
		t.Error("expected error for not-found record, got nil")
	}
}

func TestRoute53Client_GetCurrentIP_Error(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, sampleErrorResponse)
	})

	_, err := client.GetCurrentIP(context.Background())
	if err == nil {
		t.Fatal("expected error from HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "InvalidChangeBatch") {
		t.Errorf("expected parsed AWS error code in message, got: %v", err)
	}
}

func TestRoute53Client_UpdateIP(t *testing.T) {
	var bodyBytes []byte
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/rrset/") {
			t.Errorf("expected path ending in /rrset/, got %s", r.URL.Path)
		}
		bodyBytes, _ = io.ReadAll(r.Body)
		if !strings.Contains(string(bodyBytes), "<Action>UPSERT</Action>") {
			t.Errorf("expected UPSERT action in body, got %s", string(bodyBytes))
		}
		if !strings.Contains(string(bodyBytes), "<Value>5.6.7.8</Value>") {
			t.Errorf("expected new IP in body, got %s", string(bodyBytes))
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, sampleChangeResponse)
	})

	if err := client.UpdateIP(context.Background(), "5.6.7.8"); err != nil {
		t.Fatalf("UpdateIP failed: %v", err)
	}
	if !strings.Contains(string(bodyBytes), `xmlns="https://route53.amazonaws.com/doc/2013-04-01/"`) {
		t.Error("request body missing Route53 XML namespace")
	}
}

// TestRoute53Client_GetCurrentIP_EmptyHostname verifies that an empty hostname
// does not panic. Config.Validate() catches this earlier, but the client must
// stay safe.
func TestRoute53Client_GetCurrentIP_EmptyHostname(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, sampleEmptyListResponse)
	})
	client.hostname = ""

	_, err := client.GetCurrentIP(context.Background())
	if err == nil {
		t.Error("expected error for empty hostname, got nil")
	}
}

func TestRoute53Client_UpdateIP_EmptyHostname(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, sampleChangeResponse)
	})
	client.hostname = ""
	_ = client.UpdateIP(context.Background(), "1.2.3.4") // must not panic
}

func TestRoute53Client_AlreadyDottedHostname(t *testing.T) {
	var capturedName string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		capturedName = r.URL.Query().Get("name")
		_, _ = io.WriteString(w, sampleListResponse)
	})
	client.hostname = "test.example.com." // already dotted

	if _, err := client.GetCurrentIP(context.Background()); err != nil {
		t.Fatalf("GetCurrentIP failed: %v", err)
	}
	if capturedName != "test.example.com." {
		t.Errorf("expected name=%q (no double-dot), got %q", "test.example.com.", capturedName)
	}
}

func TestRoute53Client_UpdateIP_Error(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, sampleErrorResponse)
	})

	err := client.UpdateIP(context.Background(), "5.6.7.8")
	if err == nil {
		t.Fatal("expected error from HTTP 400, got nil")
	}
	if !strings.Contains(err.Error(), "InvalidChangeBatch") {
		t.Errorf("expected parsed AWS error code, got: %v", err)
	}
}

// TestRoute53Client_Auth_RejectsMissingCredentials guards the constructor's
// fail-closed behaviour.
func TestRoute53Client_Auth_RejectsMissingCredentials(t *testing.T) {
	cases := []struct {
		name, ak, sk string
	}{
		{"empty access", "", "secret"},
		{"empty secret", "access", ""},
		{"both empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRoute53Client(context.Background(), "us-east-1", tc.ak, tc.sk, "", "Z1", "h.example.com", 300)
			if err == nil {
				t.Error("expected error for missing credentials, got nil")
			}
		})
	}
}

// TestParseAWSError_ShapeMatrix covers the three body shapes
// parseAWSError handles. The function is the only thing translating
// Route53 HTTP failures into operator-facing error text, so a
// regression here turns every Route53 error into an opaque HTTP status.
func TestParseAWSError_ShapeMatrix(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantInclude []string
	}{
		{
			name:   "ErrorResponse wrapper (most common)",
			status: 400,
			body: `<?xml version="1.0" encoding="UTF-8"?>
<ErrorResponse>
  <Error>
    <Type>Sender</Type>
    <Code>InvalidChangeBatch</Code>
    <Message>Tried to create resource record set but it already exists</Message>
  </Error>
  <RequestId>abc-123</RequestId>
</ErrorResponse>`,
			wantInclude: []string{"HTTP 400", "InvalidChangeBatch", "already exists"},
		},
		{
			name:   "flat Error root (STS-style)",
			status: 403,
			body: `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>AccessDenied</Code>
  <Message>User is not authorized to perform route53:ChangeResourceRecordSets</Message>
</Error>`,
			wantInclude: []string{"HTTP 403", "AccessDenied", "not authorized"},
		},
		{
			name:        "unparseable body falls back to raw",
			status:      502,
			body:        "upstream connect error or disconnect/reset before headers. reset reason: connection timeout",
			wantInclude: []string{"HTTP 502", "upstream connect error"},
		},
		{
			name:        "empty body",
			status:      500,
			body:        "",
			wantInclude: []string{"HTTP 500"},
		},
		{
			name:        "oversized body snippet gets truncated with ellipsis",
			status:      500,
			body:        "garbage " + strings.Repeat("x", 400),
			wantInclude: []string{"HTTP 500", "..."},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseAWSError(tc.status, []byte(tc.body))
			if err == nil {
				t.Fatal("parseAWSError returned nil; all inputs should produce a non-nil error")
			}
			msg := err.Error()
			for _, want := range tc.wantInclude {
				if !strings.Contains(msg, want) {
					t.Errorf("error missing %q\n---\ngot: %s", want, msg)
				}
			}
		})
	}
}
