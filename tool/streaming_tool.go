package tool

// StreamFuncTool is a concrete implementation of StreamingFunctionTool
// that wraps a function returning deterministic chunks.
type StreamFuncTool struct {
	name        string
	description string
	decl        Declaration
	stream      func(args map[string]any) ([]StreamChunk, error)
	longRunning bool
}

func (s *StreamFuncTool) Name() string             { return s.name }
func (s *StreamFuncTool) Description() string      { return s.description }
func (s *StreamFuncTool) IsLongRunning() bool      { return s.longRunning }
func (s *StreamFuncTool) Declaration() Declaration { return cloneDeclaration(s.decl) }

func (s *StreamFuncTool) RunStream(args map[string]any) ([]StreamChunk, error) {
	return s.stream(args)
}

// NewStreamingFunctionTool creates a StreamingFunctionTool from a name,
// description, and a streaming function.
func NewStreamingFunctionTool(name, description string, stream func(args map[string]any) ([]StreamChunk, error)) StreamingFunctionTool {
	return &StreamFuncTool{
		name:        name,
		description: description,
		stream:      stream,
	}
}

// NewStreamingFunctionToolWithDeclaration creates a StreamingFunctionTool
// with an explicit Declaration.
func NewStreamingFunctionToolWithDeclaration(name, description string, decl Declaration, stream func(args map[string]any) ([]StreamChunk, error)) StreamingFunctionTool {
	return &StreamFuncTool{
		name:        name,
		description: description,
		decl:        cloneDeclaration(decl),
		stream:      stream,
	}
}

// ExecuteStream runs a streaming tool and collects the chunks into a
// normal function response (non-live mode).
func ExecuteStream(callID, name string, args map[string]any, t StreamingFunctionTool) CallResult {
	cr := CallResult{CallID: callID, Name: name}
	if t == nil {
		cr.Error = "streaming tool not found"
		cr.Result = map[string]any{"error": cr.Error}
		return cr
	}

	chunks, err := t.RunStream(args)
	if err != nil {
		cr.Error = err.Error()
		cr.Result = map[string]any{"error": err.Error()}
		return cr
	}

	result, err := CollectStreamChunks(chunks)
	if err != nil {
		cr.Error = err.Error()
	}
	cr.Result = result
	return cr
}
