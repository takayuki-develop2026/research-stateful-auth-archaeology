package run

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

type RuntimePreset string

const (
	RuntimePresetFast         RuntimePreset = "fast"
	RuntimePresetBalanced     RuntimePreset = "balanced"
	RuntimePresetHighAccuracy RuntimePreset = "high_accuracy"
	RuntimePresetCustom       RuntimePreset = "custom"
)

type EngineChoice struct {
	Kind EngineKind `json:"kind"`
}

type CapabilitySelection struct {
	Capability EngineCapability `json:"capability"`
	Choices    []EngineChoice   `json:"choices"`
}

type EngineSelection struct {
	Preset       RuntimePreset         `json:"preset"`
	Preprocess   []EngineChoice        `json:"preprocess"`
	OCR          []EngineChoice        `json:"ocr"`
	DocParse     []EngineChoice        `json:"docparse"`
	Embedding    []EngineChoice        `json:"embedding"`
	Vision       []EngineChoice        `json:"vision"`
	LLM          []EngineChoice        `json:"llm"`
	MetadataJSON map[string]any        `json:"metadata_json,omitempty"`
	Raw          map[string][]string   `json:"-"`
}

func DefaultSelectionForPreset(p RuntimePreset) EngineSelection {
	switch p {
	case RuntimePresetFast:
		return EngineSelection{
			Preset:     RuntimePresetFast,
			Preprocess: []EngineChoice{{Kind: EngineKindOpenCVBasic}},
			OCR:        []EngineChoice{{Kind: EngineKindPaddleOCR}},
			DocParse:   []EngineChoice{},
			Embedding:  []EngineChoice{{Kind: EngineKindOpenCLIP}},
			Vision:     []EngineChoice{{Kind: EngineKindQwenVL}},
			LLM:        []EngineChoice{{Kind: EngineKindGeminiFlash}},
		}
	case RuntimePresetHighAccuracy:
		return EngineSelection{
			Preset: RuntimePresetHighAccuracy,
			Preprocess: []EngineChoice{
				{Kind: EngineKindOpenCVBasic},
				{Kind: EngineKindDeblurBasic},
				{Kind: EngineKindDeskewBasic},
				{Kind: EngineKindDenoiseBasic},
			},
			OCR:       []EngineChoice{{Kind: EngineKindPaddleOCR}},
			DocParse:  []EngineChoice{{Kind: EngineKindPPStructureV3}},
			Embedding: []EngineChoice{{Kind: EngineKindOpenCLIP}},
			Vision:    []EngineChoice{{Kind: EngineKindQwenVL}},
			LLM: []EngineChoice{
				{Kind: EngineKindGPT5},
				{Kind: EngineKindGeminiFlash},
				{Kind: EngineKindClaudeHaiku45},
			},
		}
	case RuntimePresetBalanced:
		fallthrough
	default:
		return EngineSelection{
			Preset: RuntimePresetBalanced,
			Preprocess: []EngineChoice{
				{Kind: EngineKindOpenCVBasic},
				{Kind: EngineKindDeskewBasic},
			},
			OCR:       []EngineChoice{{Kind: EngineKindPaddleOCR}},
			DocParse:  []EngineChoice{{Kind: EngineKindPPStructureV3}},
			Embedding: []EngineChoice{{Kind: EngineKindOpenCLIP}},
			Vision:    []EngineChoice{{Kind: EngineKindQwenVL}},
			LLM: []EngineChoice{
				{Kind: EngineKindGemma3},
				{Kind: EngineKindGeminiFlash},
			},
		}
	}
}

func (s EngineSelection) Validate(catalog EngineCatalog) error {
	validateChoices := func(cap EngineCapability, choices []EngineChoice) error {
		for _, ch := range choices {
			d, ok := catalog.Find(ch.Kind)
			if !ok {
				return fmt.Errorf("unknown engine kind: %s", ch.Kind)
			}
			if d.Capability != cap {
				return fmt.Errorf("engine %s does not belong to capability %s", ch.Kind, cap)
			}
			if !d.Enabled {
				return fmt.Errorf("engine %s is disabled", ch.Kind)
			}
		}
		return nil
	}

	if err := validateChoices(EngineCapabilityPreprocess, s.Preprocess); err != nil {
		return err
	}
	if err := validateChoices(EngineCapabilityOCR, s.OCR); err != nil {
		return err
	}
	if err := validateChoices(EngineCapabilityDocParse, s.DocParse); err != nil {
		return err
	}
	if err := validateChoices(EngineCapabilityEmbedding, s.Embedding); err != nil {
		return err
	}
	if err := validateChoices(EngineCapabilityVision, s.Vision); err != nil {
		return err
	}
	if err := validateChoices(EngineCapabilityLLM, s.LLM); err != nil {
		return err
	}

	return nil
}

func (s EngineSelection) CanonicalJSON() (string, error) {
	type canonical struct {
		Preset     RuntimePreset `json:"preset"`
		Preprocess []string      `json:"preprocess"`
		OCR        []string      `json:"ocr"`
		DocParse   []string      `json:"docparse"`
		Embedding  []string      `json:"embedding"`
		Vision     []string      `json:"vision"`
		LLM        []string      `json:"llm"`
	}

	toKinds := func(in []EngineChoice) []string {
		out := make([]string, 0, len(in))
		for _, c := range in {
			out = append(out, string(c.Kind))
		}
		return out
	}

	c := canonical{
		Preset:     s.Preset,
		Preprocess: toKinds(s.Preprocess),
		OCR:        toKinds(s.OCR),
		DocParse:   toKinds(s.DocParse),
		Embedding:  toKinds(s.Embedding),
		Vision:     toKinds(s.Vision),
		LLM:        toKinds(s.LLM),
	}

	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func BuildSelectionFromRaw(raw map[string][]string, preset RuntimePreset) EngineSelection {
	build := func(key string) []EngineChoice {
		var out []EngineChoice
		seen := map[string]struct{}{}
		for _, v := range raw[key] {
			k := strings.TrimSpace(v)
			if k == "" {
				continue
			}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, EngineChoice{Kind: EngineKind(k)})
		}
		return out
	}

	return EngineSelection{
		Preset:     preset,
		Preprocess: build("preprocess"),
		OCR:        build("ocr"),
		DocParse:   build("docparse"),
		Embedding:  build("embedding"),
		Vision:     build("vision"),
		LLM:        build("llm"),
		Raw:        raw,
	}
}

func (s EngineSelection) HasCapability(cap EngineCapability) bool {
	switch cap {
	case EngineCapabilityPreprocess:
		return len(s.Preprocess) > 0
	case EngineCapabilityOCR:
		return len(s.OCR) > 0
	case EngineCapabilityDocParse:
		return len(s.DocParse) > 0
	case EngineCapabilityEmbedding:
		return len(s.Embedding) > 0
	case EngineCapabilityVision:
		return len(s.Vision) > 0
	case EngineCapabilityLLM:
		return len(s.LLM) > 0
	default:
		return false
	}
}

func (s EngineSelection) PrimaryChoice(cap EngineCapability) (EngineChoice, bool) {
	switch cap {
	case EngineCapabilityPreprocess:
		if len(s.Preprocess) > 0 {
			return s.Preprocess[0], true
		}
	case EngineCapabilityOCR:
		if len(s.OCR) > 0 {
			return s.OCR[0], true
		}
	case EngineCapabilityDocParse:
		if len(s.DocParse) > 0 {
			return s.DocParse[0], true
		}
	case EngineCapabilityEmbedding:
		if len(s.Embedding) > 0 {
			return s.Embedding[0], true
		}
	case EngineCapabilityVision:
		if len(s.Vision) > 0 {
			return s.Vision[0], true
		}
	case EngineCapabilityLLM:
		if len(s.LLM) > 0 {
			return s.LLM[0], true
		}
	}
	return EngineChoice{}, false
}

func OrderedKinds(choices []EngineChoice) []EngineKind {
	out := make([]EngineKind, 0, len(choices))
	for _, c := range choices {
		out = append(out, c.Kind)
	}
	return out
}

func ContainsChoice(choices []EngineChoice, kind EngineKind) bool {
	return slices.ContainsFunc(choices, func(c EngineChoice) bool {
		return c.Kind == kind
	})
}