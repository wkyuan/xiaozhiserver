package types

type AsrConfig struct {
	Provider string                 `json:"provider"`
	Config   map[string]interface{} `json:"config"`
}

type TtsConfig struct {
	Provider string                 `json:"provider"`
	Config   map[string]interface{} `json:"config"`
}

type LlmConfig struct {
	Provider string                 `json:"provider"`
	Config   map[string]interface{} `json:"config"`
}

type VadConfig struct {
	Provider string                 `json:"provider"`
	Config   map[string]interface{} `json:"config"`
}

type UConfig struct {
	SystemPrompt string    `json:"system_prompt"`
	Asr          AsrConfig `json:"asr"`
	Tts          TtsConfig `json:"tts"`
	Llm          LlmConfig `json:"llm"`
	Vad          VadConfig `json:"vad"`
}
