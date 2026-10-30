package files

import "copypaste.hk/platform/internal/core"

type NativeTextEvidence struct {
	EngineSHA256 string `json:"engine_sha256"`
	PageCount    int    `json:"page_count"`
	OCRPageCount int    `json:"ocr_page_count"`
}

func nativeTextEvidence(extractor, state string, engine *string, pages, ocr *int) (*NativeTextEvidence, error) {
	invalid := func() (*NativeTextEvidence, error) {
		return nil, core.Fail(502, "FILE_TEXT_NATIVE_OUTPUT_INVALID", "识别正文的完整页数或引擎依据不一致，请核对原件")
	}
	if extractor != DocumentTextExtractor {
		if engine != nil || pages != nil || ocr != nil {
			return invalid()
		}
		return nil, nil
	}
	if state != "ready" {
		return nil, nil
	}
	if engine == nil || len(*engine) != 64 || pages == nil || *pages < 1 || *pages > 20 || ocr == nil || *ocr != *pages {
		return invalid()
	}
	for _, r := range *engine {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return invalid()
		}
	}
	return &NativeTextEvidence{EngineSHA256: *engine, PageCount: *pages, OCRPageCount: *ocr}, nil
}
