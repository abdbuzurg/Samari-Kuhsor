// Package qr generates the batch QR codes.
//
// docs/01-DECISIONS.md D11: the client does not print its own wrappers. Wrappers
// are ordered externally against a planned batch volume, IN ADVANCE, so the QR
// code is generated in the CRM and handed to the printing company.
//
// The consequence, stated in D11 and easy to miss: QR generation is needed
// *before* the plant produces anything, not from launch onward. A code is issued
// against a planned batch that does not yet physically exist.
//
// What the payload must therefore be: a stable URL that resolves later. It cannot
// encode production data — quantities, dates, test results — because none of that
// is known when the wrapper is printed, and a printed wrapper cannot be corrected.
package qr

import (
	"fmt"
	"net/url"
	"strings"
)

// Payload is the string encoded into a batch's QR code.
//
// It is deliberately a public traceability URL and nothing else. A consumer
// scanning a jar reaches a page that looks the batch up at scan time, so the
// information stays current even though the wrapper was printed months earlier.
//
// Encoding the data directly would freeze it: a wrapper printed in August cannot
// learn that the batch was recalled in October.
func Payload(baseURL, batchNo string) (string, error) {
	batchNo = strings.TrimSpace(batchNo)
	if batchNo == "" {
		return "", fmt.Errorf("qr: batch number is required")
	}
	base, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || base.Host == "" {
		return "", fmt.Errorf("qr: invalid base URL %q", baseURL)
	}
	// Path segment, not a query parameter: it is shorter (fewer QR modules, so a
	// larger printed module size at the same physical wrapper area) and it reads
	// as a permanent address rather than a lookup.
	base.Path = base.Path + "/b/" + url.PathEscape(batchNo)
	return base.String(), nil
}

// BatchNoFromPayload recovers the batch number from a payload, for verifying an
// issued code round-trips.
func BatchNoFromPayload(payload string) (string, error) {
	u, err := url.Parse(payload)
	if err != nil {
		return "", fmt.Errorf("qr: unparseable payload: %w", err)
	}
	_, batch, found := strings.Cut(strings.TrimPrefix(u.Path, "/"), "b/")
	if !found || batch == "" {
		return "", fmt.Errorf("qr: payload %q carries no batch number", payload)
	}
	decoded, err := url.PathUnescape(batch)
	if err != nil {
		return "", fmt.Errorf("qr: undecodable batch number: %w", err)
	}
	return decoded, nil
}
