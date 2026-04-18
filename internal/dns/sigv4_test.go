package dns

import (
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestDeriveSigningKey_AWSReference validates the SigV4 key-derivation chain
// against the canonical AWS test vector documented at
// https://docs.aws.amazon.com/IAM/latest/UserGuide/signing-elements.html
func TestDeriveSigningKey_AWSReference(t *testing.T) {
	secret := "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	key := deriveSigningKey(secret, "20150830", "us-east-1", "iam")
	got := hex.EncodeToString(key)
	want := "c4afb1cc5771d871763a393e44b703571b55cc28424d1a5e86da6ed3c154a4b9"
	if got != want {
		t.Errorf("signing key mismatch\nwant: %s\n got: %s", want, got)
	}
}

func TestAwsURIEncode(t *testing.T) {
	cases := []struct {
		name, in    string
		encodeSlash bool
		want        string
	}{
		{"unreserved passes through", "AZaz09-_.~", false, "AZaz09-_.~"},
		{"slash preserved in path", "/foo/bar", false, "/foo/bar"},
		{"slash encoded in query", "/foo/bar", true, "%2Ffoo%2Fbar"},
		{"space encodes to %20", "a b", true, "a%20b"},
		{"plus encoded", "a+b", true, "a%2Bb"},
		{"equals encoded", "a=b", true, "a%3Db"},
		{"percent encoded", "a%b", true, "a%25b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := awsURIEncode(tc.in, tc.encodeSlash)
			if got != tc.want {
				t.Errorf("awsURIEncode(%q, %v) = %q, want %q", tc.in, tc.encodeSlash, got, tc.want)
			}
		})
	}
}

func TestCanonicalQueryString(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"single pair", "foo=bar", "foo=bar"},
		{"sorted by key", "b=2&a=1", "a=1&b=2"},
		{"value with dot", "name=test.example.com.&type=A", "name=test.example.com.&type=A"},
		{"value with space", "q=hello+world", "q=hello%2Bworld"},
		{"key without value", "flag", "flag="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := canonicalQueryString(tc.in)
			if got != tc.want {
				t.Errorf("canonicalQueryString(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSignRequest_SetsAuthorizationHeader confirms the signer installs the
// expected header shape. Exact signature value is not pinned (too brittle);
// presence and structural correctness are.
func TestSignRequest_SetsAuthorizationHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://route53.amazonaws.com/2013-04-01/hostedzone/Z1/rrset?name=x&type=A", nil)
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	signRequest(req, "AKIDEXAMPLE", "secret", "", "us-east-1", "route53", emptyPayloadHash, now)

	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		t.Errorf("expected AWS4-HMAC-SHA256 prefix, got: %s", auth)
	}
	for _, required := range []string{
		"Credential=AKIDEXAMPLE/20260417/us-east-1/route53/aws4_request",
		"SignedHeaders=host;x-amz-date",
		"Signature=",
	} {
		if !strings.Contains(auth, required) {
			t.Errorf("Authorization header missing %q\nfull: %s", required, auth)
		}
	}
	if req.Header.Get("X-Amz-Date") != "20260417T120000Z" {
		t.Errorf("X-Amz-Date wrong: %s", req.Header.Get("X-Amz-Date"))
	}
	// No session token → X-Amz-Security-Token must not be set (its
	// presence on the wire without being signed is a bind-time AWS error).
	if v := req.Header.Get("X-Amz-Security-Token"); v != "" {
		t.Errorf("empty sessionToken still set header: %q", v)
	}
	if strings.Contains(auth, "x-amz-security-token") {
		t.Errorf("empty sessionToken still in SignedHeaders: %s", auth)
	}
}

// TestSignRequest_WithSessionToken verifies the STS-credential path:
// X-Amz-Security-Token appears on the request, x-amz-security-token
// appears in the canonical SignedHeaders list (alphabetically after
// host and x-amz-date), and the resulting signature differs from the
// no-token version with otherwise identical inputs. This is the
// Lambda path — without it, Route53 rejects Lambda-exec-role creds.
func TestSignRequest_WithSessionToken(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	url := "https://route53.amazonaws.com/2013-04-01/hostedzone/Z1/rrset?name=x&type=A"

	// Sign the same request twice — once without token, once with —
	// and compare.
	a, _ := http.NewRequest(http.MethodGet, url, nil)
	signRequest(a, "AKIDEXAMPLE", "secret", "", "us-east-1", "route53", emptyPayloadHash, now)

	b, _ := http.NewRequest(http.MethodGet, url, nil)
	signRequest(b, "AKIDEXAMPLE", "secret", "STS-SESSION-TOKEN-FIXTURE", "us-east-1", "route53", emptyPayloadHash, now)

	if b.Header.Get("X-Amz-Security-Token") != "STS-SESSION-TOKEN-FIXTURE" {
		t.Errorf("X-Amz-Security-Token wrong: %q", b.Header.Get("X-Amz-Security-Token"))
	}
	authB := b.Header.Get("Authorization")
	if !strings.Contains(authB, "SignedHeaders=host;x-amz-date;x-amz-security-token") {
		t.Errorf("SignedHeaders missing token: %s", authB)
	}

	// Signatures must differ — if they matched, the token wasn't actually
	// part of the canonical request and Route53 would reject the call.
	if a.Header.Get("Authorization") == authB {
		t.Error("signature identical with/without session token — canonical request is not incorporating the token")
	}
}
