package transcribe

// TranscribeConfig is a structure that holds the configuration for the Transcribe method.
type TranscribeConfig struct {
	Model    string
	Language string
	File     string
}

// TranscribeOption is a function type that allows to set options for the Transcribe method.
type TranscribeOption func(*TranscribeConfig)

// WithModel sets the model for the Transcribe method.
func WithModel(model string) TranscribeOption {
	return func(tc *TranscribeConfig) {
		tc.Model = model
	}
}

// WithLanguage sets the language for the Transcribe method.
func WithLanguage(lang string) TranscribeOption {
	return func(tc *TranscribeConfig) {
		tc.Language = lang
	}
}

// WithFile sets the file for the Transcribe method.
func WithFile(file string) TranscribeOption {
	return func(tc *TranscribeConfig) {
		tc.File = file
	}
}


