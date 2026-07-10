package llm

// ResponseFormat constrains model output shape for providers that support it.
type ResponseFormat struct {
	Type       string
	Name       string
	Strict     bool
	JSONSchema map[string]any
}
