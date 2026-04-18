package dns

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// sigV4Algorithm is the fixed algorithm identifier used in the Authorization header.
const sigV4Algorithm = "AWS4-HMAC-SHA256"

// signRequest attaches an AWS Signature Version 4 Authorization header to req.
//
// Reference: https://docs.aws.amazon.com/IAM/latest/UserGuide/create-signed-request.html
//
// This is a minimal implementation covering the subset dddns needs:
//   - no presigned query strings (Authorization header form only)
//   - the caller provides the payload hash (hex SHA-256) so streaming payloads stay simple;
//     for the two request shapes dddns uses — GET with no body and POST with an XML body
//     fully buffered in memory — this is straightforward
//
// sessionToken handling (for STS temporary credentials — e.g. the creds
// Lambda injects as AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY /
// AWS_SESSION_TOKEN env vars via its execution role): when non-empty, the
// token is set as the X-Amz-Security-Token header on the request *before*
// the canonical request is built, so the header appears in both the
// alphabetically-sorted canonical-headers block and the signed-headers
// list. Long-lived IAM user credentials pass "" and get byte-identical
// behaviour to the no-token form (proven by TestSigV4 vector tests).
func signRequest(req *http.Request, accessKey, secretKey, sessionToken, region, service, payloadHash string, now time.Time) {
	timestamp := now.UTC().Format("20060102T150405Z")
	datestamp := now.UTC().Format("20060102")
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", datestamp, region, service)

	req.Header.Set("X-Amz-Date", timestamp)
	if sessionToken != "" {
		// Must be set before building canonical headers — AWS verifies that
		// x-amz-security-token appears in the signed-headers list and that
		// the on-the-wire value matches what was signed.
		req.Header.Set("X-Amz-Security-Token", sessionToken)
	}

	host := req.URL.Host
	if host == "" {
		host = req.Host
	}

	// Canonical headers are lowercase name, sorted alphabetically. "host" <
	// "x-amz-date" < "x-amz-security-token" is the required order.
	signedHeaders := "host;x-amz-date"
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-date:%s\n", host, timestamp)
	if sessionToken != "" {
		signedHeaders += ";x-amz-security-token"
		canonicalHeaders += fmt.Sprintf("x-amz-security-token:%s\n", sessionToken)
	}

	canonicalQuery := canonicalQueryString(req.URL.RawQuery)
	canonicalURI := canonicalURIPath(req.URL.Path)

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	stringToSign := strings.Join([]string{
		sigV4Algorithm,
		timestamp,
		credentialScope,
		hex.EncodeToString(sha256Sum([]byte(canonicalRequest))),
	}, "\n")

	signingKey := deriveSigningKey(secretKey, datestamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	auth := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		sigV4Algorithm, accessKey, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", auth)
}

// deriveSigningKey derives the SigV4 signing key via the canonical HMAC chain.
func deriveSigningKey(secret, datestamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Sum(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// hashBody returns the lowercase-hex SHA-256 of body. A nil body hashes to the
// empty-string digest.
func hashBody(body io.Reader) (string, []byte, error) {
	if body == nil {
		return emptyPayloadHash, nil, nil
	}
	buf, err := io.ReadAll(body)
	if err != nil {
		return "", nil, fmt.Errorf("read body for signing: %w", err)
	}
	return hex.EncodeToString(sha256Sum(buf)), buf, nil
}

// emptyPayloadHash is the SHA-256 of the empty byte slice. Precomputed for GETs.
const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// canonicalURIPath applies SigV4 URI encoding to the path. The path separator
// "/" is preserved; all other bytes outside the unreserved set are percent-
// encoded. Route53 paths only contain unreserved characters plus "/", but we
// encode defensively in case a hosted zone ID ever contains something unusual.
func canonicalURIPath(path string) string {
	if path == "" {
		return "/"
	}
	return awsURIEncode(path, false)
}

// canonicalQueryString parses the raw query string, URI-encodes each key and
// value, and emits them sorted by key. Matches the SigV4 spec.
func canonicalQueryString(raw string) string {
	if raw == "" {
		return ""
	}
	type kv struct{ k, v string }
	var pairs []kv
	for _, seg := range strings.Split(raw, "&") {
		if seg == "" {
			continue
		}
		eq := strings.IndexByte(seg, '=')
		var k, v string
		if eq < 0 {
			k = seg
		} else {
			k = seg[:eq]
			v = seg[eq+1:]
		}
		pairs = append(pairs, kv{awsURIEncode(k, true), awsURIEncode(v, true)})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
	var b strings.Builder
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(p.k)
		b.WriteByte('=')
		b.WriteString(p.v)
	}
	return b.String()
}

// awsURIEncode percent-encodes every byte outside the SigV4 unreserved set:
//
//	A-Z a-z 0-9 - _ . ~
//
// When encodeSlash is false, "/" is also left literal (for paths); when true,
// it is encoded (for query values).
func awsURIEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
