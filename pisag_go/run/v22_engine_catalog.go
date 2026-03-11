package run

type EngineCapability string

const (
	EngineCapabilityPreprocess EngineCapability = "preprocess"
	EngineCapabilityOCR        EngineCapability = "ocr"
	EngineCapabilityDocParse   EngineCapability = "docparse"
	EngineCapabilityEmbedding  EngineCapability = "embedding"
	EngineCapabilityVision     EngineCapability = "vision"
	EngineCapabilityLLM        EngineCapability = "llm"
)

type EngineKind string

const (
	// preprocess
	EngineKindOpenCVBasic  EngineKind = "opencv_basic"
	EngineKindDeblurBasic  EngineKind = "deblur_basic"
	EngineKindDeskewBasic  EngineKind = "deskew_basic"
	EngineKindDenoiseBasic EngineKind = "denoise_basic"

	// ocr
	EngineKindPaddleOCR EngineKind = "paddleocr"

	// docparse
	EngineKindPPStructureV3 EngineKind = "pp_structure_v3"

	// embedding
	EngineKindOpenCLIP EngineKind = "openclip"

	// vision
	EngineKindQwenVL EngineKind = "qwen_vl"

	// llm
	EngineKindGemma3        EngineKind = "gemma3"
	EngineKindMistralSmall  EngineKind = "mistral_small_31"
	EngineKindGPT5          EngineKind = "gpt5"
	EngineKindGeminiFlash   EngineKind = "gemini_flash"
	EngineKindClaudeHaiku45 EngineKind = "claude_haiku_45"
)

type EngineProvider string

const (
	EngineProviderLocal      EngineProvider = "local"
	EngineProviderOpenAI     EngineProvider = "openai"
	EngineProviderGoogle     EngineProvider = "google"
	EngineProviderAnthropic  EngineProvider = "anthropic"
	EngineProviderOpenSource EngineProvider = "opensource"
)

type EngineDefinition struct {
	Capability   EngineCapability
	Kind         EngineKind
	DisplayName  string
	Provider     EngineProvider
	Version      string
	Enabled      bool
	DefaultOrder int
}

type EngineCatalog struct {
	Definitions []EngineDefinition
}

func DefaultV221EngineCatalog() EngineCatalog {
	return EngineCatalog{
		Definitions: []EngineDefinition{
			{
				Capability:   EngineCapabilityPreprocess,
				Kind:         EngineKindOpenCVBasic,
				DisplayName:  "OpenCV Basic",
				Provider:     EngineProviderLocal,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 10,
			},
			{
				Capability:   EngineCapabilityPreprocess,
				Kind:         EngineKindDeblurBasic,
				DisplayName:  "Deblur Basic",
				Provider:     EngineProviderLocal,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 20,
			},
			{
				Capability:   EngineCapabilityPreprocess,
				Kind:         EngineKindDeskewBasic,
				DisplayName:  "Deskew Basic",
				Provider:     EngineProviderLocal,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 30,
			},
			{
				Capability:   EngineCapabilityPreprocess,
				Kind:         EngineKindDenoiseBasic,
				DisplayName:  "Denoise Basic",
				Provider:     EngineProviderLocal,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 40,
			},
			{
				Capability:   EngineCapabilityOCR,
				Kind:         EngineKindPaddleOCR,
				DisplayName:  "PaddleOCR",
				Provider:     EngineProviderOpenSource,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 10,
			},
			{
				Capability:   EngineCapabilityDocParse,
				Kind:         EngineKindPPStructureV3,
				DisplayName:  "PP-StructureV3",
				Provider:     EngineProviderOpenSource,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 10,
			},
			{
				Capability:   EngineCapabilityEmbedding,
				Kind:         EngineKindOpenCLIP,
				DisplayName:  "OpenCLIP",
				Provider:     EngineProviderOpenSource,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 10,
			},
			{
				Capability:   EngineCapabilityVision,
				Kind:         EngineKindQwenVL,
				DisplayName:  "Qwen VL",
				Provider:     EngineProviderOpenSource,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 10,
			},
			{
				Capability:   EngineCapabilityLLM,
				Kind:         EngineKindGemma3,
				DisplayName:  "Gemma 3",
				Provider:     EngineProviderOpenSource,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 10,
			},
			{
				Capability:   EngineCapabilityLLM,
				Kind:         EngineKindMistralSmall,
				DisplayName:  "Mistral Small 3.1",
				Provider:     EngineProviderOpenSource,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 20,
			},
			{
				Capability:   EngineCapabilityLLM,
				Kind:         EngineKindGPT5,
				DisplayName:  "GPT-5",
				Provider:     EngineProviderOpenAI,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 30,
			},
			{
				Capability:   EngineCapabilityLLM,
				Kind:         EngineKindGeminiFlash,
				DisplayName:  "Gemini 2.5 Flash",
				Provider:     EngineProviderGoogle,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 40,
			},
			{
				Capability:   EngineCapabilityLLM,
				Kind:         EngineKindClaudeHaiku45,
				DisplayName:  "Claude Haiku 4.5",
				Provider:     EngineProviderAnthropic,
				Version:      "v1",
				Enabled:      true,
				DefaultOrder: 50,
			},
		},
	}
}

func (c EngineCatalog) ListByCapability(capability EngineCapability) []EngineDefinition {
	var out []EngineDefinition
	for _, d := range c.Definitions {
		if d.Capability == capability {
			out = append(out, d)
		}
	}
	return out
}

func (c EngineCatalog) Find(kind EngineKind) (EngineDefinition, bool) {
	for _, d := range c.Definitions {
		if d.Kind == kind {
			return d, true
		}
	}
	return EngineDefinition{}, false
}

func (c EngineCatalog) IsEnabled(kind EngineKind) bool {
	d, ok := c.Find(kind)
	return ok && d.Enabled
}