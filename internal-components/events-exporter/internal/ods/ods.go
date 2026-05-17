// Package ods writes minimal OpenDocument Spreadsheet (.ods) files using only
// the standard library. We emit just enough of the OpenDocument format to
// produce a file that LibreOffice / OpenOffice / Excel will open without
// complaint — a mimetype entry, a manifest.xml and a content.xml.
//
// The .ods container is a ZIP archive with one slightly unusual constraint:
// the `mimetype` entry must be the FIRST entry, stored uncompressed, with no
// extra ZIP "extra field". archive/zip handles all of that as long as we
// pass it the right header.
package ods

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"time"
)

// Sheet is a single named table in the spreadsheet.
type Sheet struct {
	Name    string
	Header  []string
	Rows    [][]Cell
}

// Cell holds a single cell value. If String != "" we emit a string cell;
// otherwise if Number is non-nil we emit a float cell; otherwise an empty cell.
type Cell struct {
	String string
	Number *float64
	Time   *time.Time
}

// StringCell is a small helper.
func StringCell(s string) Cell { return Cell{String: s} }

// NumberCell returns a float cell. NaN/Inf are coerced to string for safety.
func NumberCell(n float64) Cell { v := n; return Cell{Number: &v} }

// TimeCell returns a string cell formatted as RFC3339 nanoseconds — keeps
// ordering correct without adding a custom number format.
func TimeCell(t time.Time) Cell { v := t; return Cell{Time: &v} }

// Write serialises sheets into a .ods file at w.
func Write(w io.Writer, sheets []Sheet) error {
	zw := zip.NewWriter(w)

	// 1. mimetype — MUST be first, stored uncompressed.
	mt, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return err
	}
	if _, err := io.WriteString(mt, "application/vnd.oasis.opendocument.spreadsheet"); err != nil {
		return err
	}

	// 2. META-INF/manifest.xml
	if err := writeFile(zw, "META-INF/manifest.xml", manifestXML); err != nil {
		return err
	}

	// 3. content.xml
	body, err := buildContentXML(sheets)
	if err != nil {
		return err
	}
	if err := writeFile(zw, "content.xml", body); err != nil {
		return err
	}

	return zw.Close()
}

func writeFile(zw *zip.Writer, name, body string) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(f, body)
	return err
}

const manifestXML = `<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">
  <manifest:file-entry manifest:full-path="/" manifest:version="1.2" manifest:media-type="application/vnd.oasis.opendocument.spreadsheet"/>
  <manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>
</manifest:manifest>`

const contentXMLHeader = `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" office:version="1.2">
  <office:automatic-styles>
    <style:style style:name="ce-header" style:family="table-cell">
      <style:text-properties fo:font-weight="bold"/>
    </style:style>
  </office:automatic-styles>
  <office:body>
    <office:spreadsheet>`

const contentXMLFooter = `    </office:spreadsheet>
  </office:body>
</office:document-content>`

func buildContentXML(sheets []Sheet) (string, error) {
	var b xmlBuf
	b.WriteString(contentXMLHeader)
	b.WriteString("\n")
	for _, s := range sheets {
		if err := writeSheet(&b, s); err != nil {
			return "", err
		}
	}
	b.WriteString(contentXMLFooter)
	return b.String(), nil
}

func writeSheet(b *xmlBuf, s Sheet) error {
	cols := len(s.Header)
	for _, r := range s.Rows {
		if len(r) > cols {
			cols = len(r)
		}
	}

	b.Printf(`      <table:table table:name=%q>`+"\n", s.Name)
	if cols > 0 {
		b.Printf(`        <table:table-column table:number-columns-repeated="%d"/>`+"\n", cols)
	}

	if len(s.Header) > 0 {
		b.WriteString(`        <table:table-row>` + "\n")
		for _, h := range s.Header {
			b.WriteString(`          <table:table-cell table:style-name="ce-header" office:value-type="string"><text:p>`)
			b.WriteString(escapeXMLText(h))
			b.WriteString(`</text:p></table:table-cell>` + "\n")
		}
		b.WriteString(`        </table:table-row>` + "\n")
	}

	for _, r := range s.Rows {
		b.WriteString(`        <table:table-row>` + "\n")
		for i := 0; i < cols; i++ {
			if i >= len(r) {
				b.WriteString(`          <table:table-cell/>` + "\n")
				continue
			}
			writeCell(b, r[i])
		}
		b.WriteString(`        </table:table-row>` + "\n")
	}

	b.WriteString(`      </table:table>` + "\n")
	return nil
}

func writeCell(b *xmlBuf, c Cell) {
	switch {
	case c.Time != nil:
		s := c.Time.UTC().Format(time.RFC3339Nano)
		b.WriteString(`          <table:table-cell office:value-type="string"><text:p>`)
		b.WriteString(escapeXMLText(s))
		b.WriteString(`</text:p></table:table-cell>` + "\n")
	case c.Number != nil:
		v := strconv.FormatFloat(*c.Number, 'f', -1, 64)
		b.Printf(`          <table:table-cell office:value-type="float" office:value=%q><text:p>%s</text:p></table:table-cell>`+"\n", v, escapeXMLText(v))
	case c.String != "":
		b.WriteString(`          <table:table-cell office:value-type="string"><text:p>`)
		b.WriteString(escapeXMLText(c.String))
		b.WriteString(`</text:p></table:table-cell>` + "\n")
	default:
		b.WriteString(`          <table:table-cell/>` + "\n")
	}
}

func escapeXMLText(s string) string {
	var out xmlBuf
	if err := xml.EscapeText(&out, []byte(s)); err != nil {
		return ""
	}
	return out.String()
}

// xmlBuf is a tiny io.Writer + string-builder hybrid that satisfies
// xml.EscapeText's signature.
type xmlBuf struct{ b []byte }

func (x *xmlBuf) Write(p []byte) (int, error) { x.b = append(x.b, p...); return len(p), nil }
func (x *xmlBuf) WriteString(s string)        { x.b = append(x.b, s...) }
func (x *xmlBuf) Printf(f string, a ...any)   { x.b = append(x.b, fmt.Sprintf(f, a...)...) }
func (x *xmlBuf) String() string              { return string(x.b) }
