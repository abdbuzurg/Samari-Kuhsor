package qr

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	qrcode "github.com/yeqown/go-qrcode/v2"
)

// Images are generated on demand and never stored (docs/07-IMPLEMENTATION-PLAN.md
// I17): the payload in batches.qr_payload is the artefact of record, an image is
// a rendering of it, and a stored PNG is one more thing to back up and to go
// stale.

// SVG renders a payload as a standalone SVG.
//
// SVG rather than PNG because the printing company scales the code to the wrapper
// die: a raster at the wrong size either pixelates or costs a re-request, and a
// print house asked for "a QR code" expects vector artwork.
func SVG(payload string, moduleSize int) ([]byte, error) {
	if moduleSize <= 0 {
		moduleSize = 8
	}

	// Medium error correction: ~15% recoverable. High would survive more wrapper
	// damage but needs more modules in the same area, making each one smaller and
	// harder for a phone camera on a curved bottle. Medium is the usual choice for
	// print on packaging.
	code, err := qrcode.NewWith(payload,
		qrcode.WithEncodingMode(qrcode.EncModeByte),
		qrcode.WithErrorCorrectionLevel(qrcode.ErrorCorrectionMedium),
	)
	if err != nil {
		return nil, fmt.Errorf("qr: encode: %w", err)
	}

	w := &svgWriter{moduleSize: moduleSize}
	if err := code.Save(w); err != nil {
		return nil, fmt.Errorf("qr: render: %w", err)
	}
	return w.buf.Bytes(), nil
}

// svgWriter implements the library's Writer interface, emitting SVG rather than
// a raster.
type svgWriter struct {
	buf        bytes.Buffer
	moduleSize int
}

func (w *svgWriter) Write(mat qrcode.Matrix) error {
	dim := mat.Width()
	// Four modules of quiet zone on every side. The QR spec requires it and print
	// houses do not add it: without the margin, scanners fail against a busy
	// wrapper background.
	const quiet = 4
	size := (dim + quiet*2) * w.moduleSize

	fmt.Fprintf(&w.buf,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" shape-rendering="crispEdges">`,
		size, size, size, size)
	fmt.Fprintf(&w.buf, `<rect width="%d" height="%d" fill="#ffffff"/>`, size, size)

	var paths strings.Builder
	mat.Iterate(qrcode.IterDirection_ROW, func(x int, y int, s qrcode.QRValue) {
		if s.IsSet() {
			fmt.Fprintf(&paths, "M%d %dh%dv%dh-%dz",
				(x+quiet)*w.moduleSize, (y+quiet)*w.moduleSize,
				w.moduleSize, w.moduleSize, w.moduleSize)
		}
	})
	// One path for every dark module: a few hundred <rect> elements would balloon
	// the file and some print RIPs choke on them.
	fmt.Fprintf(&w.buf, `<path d="%s" fill="#000000"/>`, paths.String())
	w.buf.WriteString(`</svg>`)
	return nil
}

func (w *svgWriter) Close() error { return nil }

// ExportRow is one batch in the printer handoff.
type ExportRow struct {
	BatchNo  string
	ItemSKU  string
	ItemName string
	Payload  string
}

// Export writes a ZIP containing one SVG per batch plus a CSV manifest.
//
// The manifest exists because the printer receives a folder of files and needs to
// know which code belongs to which product without opening them. Its row count is
// asserted against the file count in tests: a silently short export would mean a
// batch shipped with the wrong wrapper, which is a traceability failure that
// cannot be fixed after printing.
func Export(w io.Writer, rows []ExportRow, moduleSize int) error {
	if len(rows) == 0 {
		return fmt.Errorf("qr: nothing to export")
	}

	zw := zip.NewWriter(w)

	var manifest bytes.Buffer
	csvw := csv.NewWriter(&manifest)
	// Semicolon-delimited with a UTF-8 BOM: Excel in a Russian locale opens a
	// comma-separated UTF-8 file as one column of mojibake, and the printer will
	// open this in Excel.
	csvw.Comma = ';'
	manifest.WriteString("\xEF\xBB\xBF")
	if err := csvw.Write([]string{"batch_no", "sku", "name", "payload", "file"}); err != nil {
		return fmt.Errorf("qr: manifest header: %w", err)
	}

	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		if seen[r.BatchNo] {
			return fmt.Errorf("qr: batch %s appears twice in the export", r.BatchNo)
		}
		seen[r.BatchNo] = true

		filename := "qr/" + safeFilename(r.BatchNo) + ".svg"
		svg, err := SVG(r.Payload, moduleSize)
		if err != nil {
			return fmt.Errorf("qr: batch %s: %w", r.BatchNo, err)
		}
		f, err := zw.Create(filename)
		if err != nil {
			return fmt.Errorf("qr: zip entry %s: %w", filename, err)
		}
		if _, err := f.Write(svg); err != nil {
			return fmt.Errorf("qr: write %s: %w", filename, err)
		}
		if err := csvw.Write([]string{r.BatchNo, r.ItemSKU, r.ItemName, r.Payload, filename}); err != nil {
			return fmt.Errorf("qr: manifest row: %w", err)
		}
	}

	csvw.Flush()
	if err := csvw.Error(); err != nil {
		return fmt.Errorf("qr: manifest: %w", err)
	}
	mf, err := zw.Create("manifest.csv")
	if err != nil {
		return fmt.Errorf("qr: manifest entry: %w", err)
	}
	if _, err := mf.Write(manifest.Bytes()); err != nil {
		return fmt.Errorf("qr: write manifest: %w", err)
	}

	return zw.Close()
}

// safeFilename keeps a batch number usable as a filename on any platform the
// printer might be on.
func safeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "batch"
	}
	return b.String()
}
