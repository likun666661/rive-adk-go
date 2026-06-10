package retryreflect

import (
	"fmt"
	"sync"

	"github.com/likun666661/rive-adk-go/callbackctx"
	"github.com/likun666661/rive-adk-go/plugin"
)

type Config struct {
	Name       string
	MaxRetries int
}

type Plugin struct {
	*plugin.Plugin
	mu         sync.Mutex
	failures   map[string]int
	maxRetries int
}

func New(cfg Config) *Plugin {
	p := &Plugin{
		failures:   make(map[string]int),
		maxRetries: cfg.MaxRetries,
	}
	name := cfg.Name
	if name == "" {
		name = "retry_reflect"
	}

	p.Plugin = plugin.New(plugin.Config{
		Name: name,
		OnToolError: func(tctx callbackctx.ToolContext, toolName string, args map[string]any, originalErr error) (map[string]any, error) {
			p.mu.Lock()
			count := p.failures[toolName] + 1
			p.failures[toolName] = count
			p.mu.Unlock()

			result := map[string]any{
				"error": originalErr.Error(),
			}

			if count <= p.maxRetries {
				result["reflection"] = fmt.Sprintf(
					"Tool %q failed (attempt %d/%d): %s. Consider an alternative approach or retry.",
					toolName, count, p.maxRetries, originalErr.Error(),
				)
			} else {
				result["reflection_exceeded"] = fmt.Sprintf(
					"Tool %q has failed %d times (max %d). Stop using this tool and adopt a different strategy.",
					toolName, count, p.maxRetries,
				)
			}

			return result, nil
		},
		AfterTool: func(tctx callbackctx.ToolContext, toolName string, args, result map[string]any, runErr error) (map[string]any, error) {
			if runErr == nil && result != nil {
				if _, hasErr := result["error"]; !hasErr {
					p.mu.Lock()
					delete(p.failures, toolName)
					p.mu.Unlock()
				}
			}
			return nil, nil
		},
	})

	return p
}

func (p *Plugin) FailureCount(toolName string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.failures[toolName]
}
