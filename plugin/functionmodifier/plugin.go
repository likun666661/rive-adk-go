package functionmodifier

import (
	"fmt"
	"maps"

	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/model"
	"github.com/likun666661/rive-adk-go/plugin"
	"github.com/likun666661/rive-adk-go/tool"
)

type Predicate func(toolName string) bool

type Config struct {
	Name       string
	Predicate  Predicate
	HiddenArgs map[string]any
}

type Plugin struct {
	*plugin.Plugin
	config Config
}

func New(cfg Config) *Plugin {
	name := cfg.Name
	if name == "" {
		name = "function_call_modifier"
	}
	p := &Plugin{config: cfg}

	p.Plugin = plugin.New(plugin.Config{
		Name: name,
		BeforeModel: func(cctx callbackctx.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
			if req == nil || cfg.Predicate == nil || len(cfg.HiddenArgs) == 0 {
				return nil, nil
			}
			for i, declAny := range req.ToolDeclarations {
				decl, ok := declAny.(tool.Declaration)
				if !ok {
					continue
				}
				if !cfg.Predicate(decl.Name) {
					continue
				}
				if decl.InputSchema == nil {
					decl.InputSchema = map[string]any{
						"type":       "object",
						"properties": make(map[string]any),
					}
				}
				props, _ := decl.InputSchema["properties"].(map[string]any)
				if props == nil {
					props = make(map[string]any)
					decl.InputSchema["properties"] = props
				}
				for k, v := range cfg.HiddenArgs {
					props[k] = v
				}
				if reqFields, ok := decl.InputSchema["required"].([]any); ok {
					for k := range cfg.HiddenArgs {
						has := false
						for _, rf := range reqFields {
							if rf == k {
								has = true
								break
							}
						}
						if !has {
							reqFields = append(reqFields, k)
						}
					}
					decl.InputSchema["required"] = reqFields
				}
				req.ToolDeclarations[i] = decl
			}
			return nil, nil
		},
		AfterModel: func(cctx callbackctx.CallbackContext, req *model.LLMRequest, resp *model.LLMResponse, callErr error) (*model.LLMResponse, error) {
			if resp == nil || resp.Content == nil || callErr != nil || cfg.Predicate == nil || cctx == nil {
				return nil, nil
			}
			state := cctx.State()
			for i, part := range resp.Content.Parts {
				fc := part.FunctionCall
				if fc == nil {
					continue
				}
				if !cfg.Predicate(fc.Name) {
					continue
				}
				stripped := make(map[string]any)
				for k := range cfg.HiddenArgs {
					if v, exists := fc.Args[k]; exists {
						stripped[k] = v
						delete(fc.Args, k)
					}
				}
				if len(stripped) > 0 && state != nil {
					for k, v := range stripped {
						key := fmt.Sprintf("hidden/%s/%s", fc.ID, k)
						state.Set(key, v)
					}
				}
				resp.Content.Parts[i].FunctionCall = fc
			}
			return nil, nil
		},
	})

	return p
}

func (p *Plugin) HiddenArgs() map[string]any {
	return maps.Clone(p.config.HiddenArgs)
}
