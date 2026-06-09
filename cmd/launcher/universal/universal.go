// Package universal provides an umbrella launcher that routes to
// sublaunchers by keyword.
//
// The first argv token is interpreted as a sublauncher keyword. If it
// matches, that sublauncher parses the remaining args and runs. With no
// args, the first registered sublauncher is used as the default. Unknown
// commands produce an error.
package universal

import (
	"context"
	"fmt"
	"strings"

	"github.com/likun666661/rive-adk-go/cmd/launcher"
)

type uniLauncher struct {
	chosen      launcher.SubLauncher
	sublauncher []launcher.SubLauncher
}

// New creates a universal Launcher that routes to the given sublaunchers.
// The first sublauncher in the list is the default when no args are provided.
func New(sublauncher ...launcher.SubLauncher) launcher.Launcher {
	return &uniLauncher{sublauncher: sublauncher}
}

// Execute implements launcher.Launcher. It parses args and runs the
// chosen sublauncher. Returns an error if there are unparsed args.
func (l *uniLauncher) Execute(ctx context.Context, config *launcher.Config, args []string) error {
	remaining, err := l.parse(args)
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		return fmt.Errorf("universal: unparsed arguments: %v", remaining)
	}
	return l.chosen.Run(ctx, config)
}

// parse matches the first argv token to a sublauncher keyword.
// Returns the remaining args after the sublauncher's Parse.
func (l *uniLauncher) parse(args []string) ([]string, error) {
	if len(l.sublauncher) == 0 {
		return nil, fmt.Errorf("universal: no sublaunchers registered")
	}

	kw := make(map[string]launcher.SubLauncher, len(l.sublauncher))
	for _, sl := range l.sublauncher {
		if _, exists := kw[sl.Keyword()]; exists {
			return nil, fmt.Errorf("universal: duplicate sublauncher keyword %q", sl.Keyword())
		}
		kw[sl.Keyword()] = sl
	}

	l.chosen = l.sublauncher[0]

	if len(args) == 0 {
		return l.chosen.Parse(nil)
	}

	key := args[0]
	if matched, ok := kw[key]; ok {
		l.chosen = matched
		return l.chosen.Parse(args[1:])
	}

	return nil, fmt.Errorf("universal: unknown command %q (available: %s)", key, l.availableKeywords())
}

func (l *uniLauncher) availableKeywords() string {
	keys := make([]string, len(l.sublauncher))
	for i, sl := range l.sublauncher {
		keys[i] = sl.Keyword()
	}
	return strings.Join(keys, ", ")
}

// CommandLineSyntax implements launcher.Launcher.
func (l *uniLauncher) CommandLineSyntax() string {
	if len(l.sublauncher) == 0 {
		return "universal: no sublaunchers registered"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Arguments: specify one of the following sublauncher keywords:\n")
	for _, sl := range l.sublauncher {
		fmt.Fprintf(&b, "  %-12s — %s\n", sl.Keyword(), sl.SimpleDescription())
	}
	fmt.Fprintf(&b, "\nWith no args, the first sublauncher (%q) is used as the default.\n",
		l.sublauncher[0].Keyword())
	fmt.Fprintf(&b, "\nDetails:\n")
	for _, sl := range l.sublauncher {
		fmt.Fprintf(&b, "  %s\n    %s\n", sl.Keyword(), sl.CommandLineSyntax())
	}
	return b.String()
}
