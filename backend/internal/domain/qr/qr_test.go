package qr

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"io"
	"strings"
	"testing"
)

// docs/01-DECISIONS.md D11 — wrappers are ordered externally against a planned
// batch volume, IN ADVANCE. A printed wrapper cannot be corrected, so everything
// here is tested against that: the payload must round-trip, the export must not
// silently drop a batch, and a code must render at a size a print house can use.

func TestPayloadRoundTrips(t *testing.T) {
	t.Parallel()

	payload, err := Payload("https://samari-kuhsor.tj", "B-2617")
	if err != nil {
		t.Fatalf("Payload: %v", err)
	}
	if payload != "https://samari-kuhsor.tj/b/B-2617" {
		t.Errorf("payload = %q", payload)
	}

	back, err := BatchNoFromPayload(payload)
	if err != nil {
		t.Fatalf("BatchNoFromPayload: %v", err)
	}
	if back != "B-2617" {
		t.Errorf("round-tripped to %q, want B-2617", back)
	}
}

// The payload is a URL, not encoded production data. A wrapper printed in August
// cannot learn that the batch was recalled in October, so the code must resolve
// at scan time rather than carry a frozen snapshot.
func TestPayloadCarriesNoProductionData(t *testing.T) {
	t.Parallel()
	payload, err := Payload("https://samari-kuhsor.tj", "B-2617")
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"status", "released", "quantity", "expires", "2026"} {
		if strings.Contains(strings.ToLower(payload), leaked) {
			t.Errorf("payload encodes %q; it must be a URL that resolves later, not a snapshot", leaked)
		}
	}
}

func TestPayloadValidation(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ base, batch string }{
		"empty batch": {"https://samari-kuhsor.tj", ""},
		"blank batch": {"https://samari-kuhsor.tj", "   "},
		"no host":     {"not-a-url", "B-2617"},
		"empty base":  {"", "B-2617"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Payload(tc.base, tc.batch); err == nil {
				t.Error("expected an error")
			}
		})
	}

	// A trailing slash on the configured base must not produce a double slash:
	// two URLs for the same batch would fragment analytics and look wrong printed.
	a, _ := Payload("https://samari-kuhsor.tj/", "B-2617")
	b, _ := Payload("https://samari-kuhsor.tj", "B-2617")
	if a != b {
		t.Errorf("trailing slash changed the payload: %q vs %q", a, b)
	}
}

// Batch numbers with characters that need escaping must still round-trip, or a
// scan resolves to the wrong page.
func TestPayloadEscapesBatchNumbers(t *testing.T) {
	t.Parallel()
	for _, batchNo := range []string{"B-2617", "B/2617", "B 2617", "Б-2617"} {
		payload, err := Payload("https://samari-kuhsor.tj", batchNo)
		if err != nil {
			t.Fatalf("%s: %v", batchNo, err)
		}
		back, err := BatchNoFromPayload(payload)
		if err != nil {
			t.Fatalf("%s: %v", batchNo, err)
		}
		if back != batchNo {
			t.Errorf("%q round-tripped to %q via %q", batchNo, back, payload)
		}
	}
}

func TestSVGIsValidAndSized(t *testing.T) {
	t.Parallel()

	svg, err := SVG("https://samari-kuhsor.tj/b/B-2617", 8)
	if err != nil {
		t.Fatalf("SVG: %v", err)
	}
	s := string(svg)

	if !strings.HasPrefix(s, `<svg xmlns="http://www.w3.org/2000/svg"`) {
		t.Errorf("not an SVG document: %.80s", s)
	}
	if !strings.HasSuffix(s, "</svg>") {
		t.Error("SVG is not closed — a truncated file would print as nothing")
	}
	// A white background rect: printed onto a coloured wrapper without it, the
	// quiet zone is not actually quiet and scanners fail.
	if !strings.Contains(s, `fill="#ffffff"`) {
		t.Error("no white background")
	}
	if !strings.Contains(s, `fill="#000000"`) {
		t.Error("no dark modules")
	}
	// crispEdges stops the RIP anti-aliasing module boundaries into grey.
	if !strings.Contains(s, `shape-rendering="crispEdges"`) {
		t.Error("missing crispEdges")
	}
	// One path, not hundreds of rects.
	if strings.Count(s, "<path") != 1 {
		t.Errorf("expected exactly one path, got %d", strings.Count(s, "<path"))
	}
}

// The quiet zone is required by the QR spec and print houses do not add it.
// Without it, scanning fails against a busy wrapper background.
func TestSVGIncludesTheQuietZone(t *testing.T) {
	t.Parallel()

	const moduleSize = 10
	svg, err := SVG("https://samari-kuhsor.tj/b/B-2617", moduleSize)
	if err != nil {
		t.Fatal(err)
	}
	s := string(svg)

	// The canvas must be 8 modules wider than the code itself (4 each side).
	// Extracting the declared width lets us assert that without re-deriving the
	// QR version here.
	width := attr(t, s, "width")
	if width%moduleSize != 0 {
		t.Errorf("width %d is not a whole number of modules", width)
	}
	// A version-2 code is 25 modules; with the quiet zone at least 33.
	if width < 33*moduleSize {
		t.Errorf("width %d looks too small to include a 4-module quiet zone", width)
	}

	// The first dark module must not sit at the very edge.
	if strings.Contains(s, `d="M0 0`) {
		t.Error("a dark module starts at 0,0 — the quiet zone is missing")
	}
}

func TestSVGModuleSizeIsHonoured(t *testing.T) {
	t.Parallel()
	small, err := SVG("https://samari-kuhsor.tj/b/B-2617", 4)
	if err != nil {
		t.Fatal(err)
	}
	large, err := SVG("https://samari-kuhsor.tj/b/B-2617", 16)
	if err != nil {
		t.Fatal(err)
	}
	if attr(t, string(large), "width") != 4*attr(t, string(small), "width") {
		t.Error("module size does not scale the canvas proportionally")
	}

	// A zero or negative size must fall back rather than emit a zero-size image
	// the printer would silently drop.
	zero, err := SVG("https://samari-kuhsor.tj/b/B-2617", 0)
	if err != nil {
		t.Fatal(err)
	}
	if attr(t, string(zero), "width") <= 0 {
		t.Error("module size 0 produced a zero-width image")
	}
}

func TestExportProducesOneFilePerBatchPlusAManifest(t *testing.T) {
	t.Parallel()

	rows := []ExportRow{
		{BatchNo: "B-2617", ItemSKU: "APJ-1000", ItemName: "Яблочный сок", Payload: "https://samari-kuhsor.tj/b/B-2617"},
		{BatchNo: "B-2618", ItemSKU: "WAT-500", ItemName: "Вода 0,5 л", Payload: "https://samari-kuhsor.tj/b/B-2618"},
		{BatchNo: "B-2619", ItemSKU: "TOM-500", ItemName: "Томатная паста", Payload: "https://samari-kuhsor.tj/b/B-2619"},
	}

	var buf bytes.Buffer
	if err := Export(&buf, rows, 8); err != nil {
		t.Fatalf("Export: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("the archive is not readable: %v", err)
	}

	var svgs int
	var manifest []byte
	for _, f := range zr.File {
		switch {
		case f.Name == "manifest.csv":
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			manifest, _ = io.ReadAll(rc)
			rc.Close()
		case strings.HasSuffix(f.Name, ".svg"):
			svgs++
		}
	}

	if svgs != len(rows) {
		t.Errorf("%d SVGs for %d batches — a short export means a batch ships with the wrong wrapper",
			svgs, len(rows))
	}
	if manifest == nil {
		t.Fatal("no manifest: the printer cannot tell which code belongs to which product")
	}

	// Excel in a Russian locale needs the BOM and a semicolon delimiter, and the
	// printer will open this in Excel.
	if !bytes.HasPrefix(manifest, []byte("\xEF\xBB\xBF")) {
		t.Error("manifest has no UTF-8 BOM; Excel will render Cyrillic as mojibake")
	}

	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(manifest, []byte("\xEF\xBB\xBF"))))
	r.Comma = ';'
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("manifest is not valid CSV: %v", err)
	}
	// Header plus one row per batch. This equality is the whole safety property.
	if len(records) != len(rows)+1 {
		t.Fatalf("manifest has %d rows for %d batches", len(records)-1, len(rows))
	}
	if strings.Join(records[0], ",") != "batch_no,sku,name,payload,file" {
		t.Errorf("manifest header = %v", records[0])
	}
	// Cyrillic product names must survive.
	if !strings.Contains(string(manifest), "Яблочный сок") {
		t.Error("Cyrillic product name did not survive into the manifest")
	}
}

// A duplicate batch in one export would overwrite a file and silently ship one
// fewer code than the manifest claims.
func TestExportRefusesDuplicateBatches(t *testing.T) {
	t.Parallel()
	rows := []ExportRow{
		{BatchNo: "B-2617", ItemSKU: "APJ-1000", Payload: "https://samari-kuhsor.tj/b/B-2617"},
		{BatchNo: "B-2617", ItemSKU: "APJ-1000", Payload: "https://samari-kuhsor.tj/b/B-2617"},
	}
	var buf bytes.Buffer
	if err := Export(&buf, rows, 8); err == nil {
		t.Fatal("a duplicate batch was accepted")
	}
}

func TestExportRefusesAnEmptySet(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := Export(&buf, nil, 8); err == nil {
		t.Fatal("an empty export was accepted; the printer would receive an empty archive")
	}
}

// Batch numbers become filenames. A slash would create a directory and a Cyrillic
// name can confuse a Windows print workstation, so they are sanitised — but the
// manifest still carries the true batch number.
func TestExportSanitisesFilenames(t *testing.T) {
	t.Parallel()
	rows := []ExportRow{
		{BatchNo: "B/2617 ю", ItemSKU: "APJ-1000", Payload: "https://samari-kuhsor.tj/b/x"},
	}
	var buf bytes.Buffer
	if err := Export(&buf, rows, 8); err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".svg") {
			if strings.ContainsAny(strings.TrimPrefix(f.Name, "qr/"), "/ ю") {
				t.Errorf("unsafe filename: %q", f.Name)
			}
		}
		if f.Name == "manifest.csv" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			// The true batch number must still be recoverable.
			if !strings.Contains(string(b), "B/2617 ю") {
				t.Error("the manifest lost the real batch number")
			}
		}
	}
}

func TestSafeFilename(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"B-2617":  "B-2617",
		"B/2617":  "B_2617",
		"B 2617":  "B_2617",
		"Б-2617":  "_-2617", // iterated per rune, not per byte
		"":        "batch",
		"../../x": "______x",
	} {
		if got := safeFilename(in); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

// attr reads an integer attribute out of the SVG header.
func attr(t *testing.T, svg, name string) int {
	t.Helper()
	marker := name + `="`
	i := strings.Index(svg, marker)
	if i < 0 {
		t.Fatalf("no %s attribute", name)
	}
	rest := svg[i+len(marker):]
	end := strings.Index(rest, `"`)
	n := 0
	for _, r := range rest[:end] {
		if r < '0' || r > '9' {
			t.Fatalf("%s is not numeric: %q", name, rest[:end])
		}
		n = n*10 + int(r-'0')
	}
	return n
}
