package files

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"mime"
	"path"
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
)

const TextExtractor = "text-v1"
const TextSourceMaxBytes int64 = 8 << 20
const TextOutputMaxBytes = 2 << 20

// Keep text-v1 derivations immutable. New OOXML formats use a separate identity
// so previously unsupported terminal rows remain historical evidence.
func TextExtractorFor(mediaType string) string {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0]))
	if base == xlsxMediaType || base == pptxMediaType {
		return OfficeTextExtractor
	}
	if base == docxMediaType {
		return DOCXTextExtractor
	}
	return TextExtractor
}

func textError(code string) error {
	return core.Fail(422, "FILE_TEXT_"+code, "此版本未能生成可检索正文；原件及其版本不变")
}

func textFormat(mediaType string, size int64) (string, error) {
	base, params, err := mime.ParseMediaType(mediaType)
	if err != nil || size < 1 {
		return "", textError("FORMAT_UNSUPPORTED")
	}
	if base == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || base == xlsxMediaType || base == pptxMediaType {
		if size > TextSourceMaxBytes {
			return "", textError("SIZE_UNSUPPORTED")
		}
		if base == xlsxMediaType {
			return "xlsx", nil
		}
		if base == pptxMediaType {
			return "pptx", nil
		}
		return "docx", nil
	}
	if base != "text/plain" && base != "text/markdown" && base != "text/csv" && base != "application/json" {
		return "", textError("FORMAT_UNSUPPORTED")
	}
	if params["charset"] != "" && !strings.EqualFold(params["charset"], "utf-8") {
		return "", textError("ENCODING_UNSUPPORTED")
	}
	if size > AgentTextMaxBytes {
		return "", textError("SIZE_UNSUPPORTED")
	}
	return base, nil
}

// ExtractText accepts already hash-verified bounded bytes. It does no filesystem,
// shell, network, macro, relationship or embedded-object execution.
func ExtractText(ctx context.Context, mediaType string, body []byte) (string, error) {
	return extractTextWith(ctx, mediaType, body, TextExtractorFor(mediaType))
}

func extractTextWith(ctx context.Context, mediaType string, body []byte, extractor string) (string, error) {
	format, err := textFormat(mediaType, int64(len(body)))
	if err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if format == "docx" {
		if extractor == DOCXTextExtractor {
			return extractComplexDOCX(ctx, body)
		}
		return extractDOCX(ctx, body)
	}
	if format == "xlsx" {
		return extractXLSX(ctx, body)
	}
	if format == "pptx" {
		return extractPPTX(ctx, body)
	}
	if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return "", textError("ENCODING_UNSUPPORTED")
	}
	text := strings.TrimPrefix(string(body), "\ufeff")
	if format == "application/json" && !json.Valid([]byte(text)) {
		return "", textError("DOCUMENT_INVALID")
	}
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), nil
}

func extractDOCX(ctx context.Context, body []byte) (string, error) {
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil || len(z.File) > 512 {
		return "", textError("DOCUMENT_INVALID")
	}
	var total uint64
	var document *zip.File
	seen := map[string]bool{}
	for _, file := range z.File {
		name := file.Name
		if seen[name] || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || path.Clean(name) != strings.TrimSuffix(name, "/") || strings.HasPrefix(name, "../") || name == ".." || file.Flags&1 != 0 {
			return "", textError("DOCUMENT_INVALID")
		}
		seen[name] = true
		if strings.Contains(strings.ToLower(name), "vbaproject") {
			return "", textError("MACRO_UNSUPPORTED")
		}
		if file.UncompressedSize64 > 64<<20 || total > (64<<20)-file.UncompressedSize64 {
			return "", textError("EXPANSION_LIMIT")
		}
		total += file.UncompressedSize64
		if file.UncompressedSize64 > 1<<20 && (file.CompressedSize64 == 0 || file.UncompressedSize64/file.CompressedSize64 > 100) {
			return "", textError("EXPANSION_LIMIT")
		}
		if name == "word/document.xml" {
			document = file
		}
	}
	if document == nil || !seen["[Content_Types].xml"] || document.UncompressedSize64 > 8<<20 {
		return "", textError("DOCUMENT_INVALID")
	}
	reader, err := document.Open()
	if err != nil {
		return "", textError("DOCUMENT_INVALID")
	}
	defer reader.Close()
	decoder := xml.NewDecoder(io.LimitReader(reader, (8<<20)+1))
	var out strings.Builder
	depth, textDepth, tokens := 0, 0, 0
	root := false
	rootSeen := false
	word := func(space string) bool {
		return space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" || space == "http://purl.oclc.org/ooxml/wordprocessingml/main"
	}
	appendText := func(s string) error {
		if out.Len()+len(s) > TextOutputMaxBytes || strings.IndexByte(s, 0) >= 0 {
			return textError("OUTPUT_LIMIT")
		}
		out.WriteString(s)
		return nil
	}
	for {
		if tokens++; tokens > 500000 {
			return "", textError("PARSE_LIMIT")
		}
		if err = ctx.Err(); err != nil {
			return "", err
		}
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", textError("DOCUMENT_INVALID")
		}
		switch t := token.(type) {
		case xml.Directive:
			return "", textError("DOCUMENT_INVALID")
		case xml.StartElement:
			depth++
			if depth > 64 {
				return "", textError("PARSE_LIMIT")
			}
			if depth == 1 {
				if rootSeen {
					return "", textError("DOCUMENT_INVALID")
				}
				rootSeen = true
				root = word(t.Name.Space) && t.Name.Local == "document"
			}
			if word(t.Name.Space) {
				if t.Name.Local == "t" {
					textDepth = depth
				}
				if t.Name.Local == "tab" {
					err = appendText("\t")
				}
				if t.Name.Local == "br" || t.Name.Local == "cr" {
					err = appendText("\n")
				}
			}
		case xml.EndElement:
			if word(t.Name.Space) && t.Name.Local == "p" {
				err = appendText("\n")
			}
			if depth == textDepth {
				textDepth = 0
			}
			depth--
		case xml.CharData:
			if textDepth > 0 {
				err = appendText(string(t))
			}
		}
		if err != nil {
			return "", err
		}
	}
	if !root || depth != 0 || !utf8.ValidString(out.String()) {
		return "", textError("DOCUMENT_INVALID")
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
