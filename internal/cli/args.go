package cli

import (
	"strings"

	"github.com/brentkeller/waid/internal/errs"
)

// booleanFlags never take a value, so they cannot swallow the argument that follows them.
var booleanFlags = map[string]bool{
	"json":     true,
	"no-sync":  true,
	"full":     true,
	"last":     true,
	"all":      true,
	"top":      true,
	"off":      true,
	"heading":  true,
	"headings": true,
	"help":     true,
	"version":  true,
}

// aliases are short forms, expanded to their long name before anything else looks at them.
var aliases = map[string]string{"p": "project", "t": "tag", "h": "help"}

// retired flags were renamed rather than dropped. The old spelling is refused so an invocation
// written against it fails loudly instead of parsing into a flag nothing reads.
var retired = map[string]string{"project": "-p for a parent, or --origin for a path"}

// repeatable flags collect every occurrence into a list instead of overwriting.
var repeatable = map[string]bool{"tag": true, "remove": true}

// Flags holds parsed command-line flags. Booleans, single values, and repeated values are kept
// apart so a flag's presence stays distinguishable from an empty value.
type Flags struct {
	bools  map[string]bool
	values map[string]string
	lists  map[string][]string
}

// Bool reads a boolean flag. An unpassed flag reads as false.
func (f Flags) Bool(name string) bool { return f.bools[name] }

// String reads a flag carrying a single value, reporting whether it was passed at all.
func (f Flags) String(name string) (string, bool) {
	value, ok := f.values[name]
	return value, ok
}

// List reads a repeatable flag, tolerating the single-value form.
func (f Flags) List(name string) []string {
	if values, ok := f.lists[name]; ok {
		return values
	}
	if value, ok := f.values[name]; ok {
		return []string{value}
	}
	return nil
}

// count reports how many flags were passed.
func (f Flags) count() int { return len(f.bools) + len(f.values) + len(f.lists) }

// ParsedArgv is argv split into a command, its positionals, and its flags.
type ParsedArgv struct {
	// Command is the first positional, empty when argv carries no command.
	Command string
	// Args holds the positionals after the command, in order.
	Args  []string
	Flags Flags
}

// ParseArgv splits argv into a command, its positionals and its flags. Unrecognised long flags are
// treated as value flags, so new commands can take options without touching the tables above.
func ParseArgv(argv []string) (ParsedArgv, error) {
	flags := Flags{
		bools:  map[string]bool{},
		values: map[string]string{},
		lists:  map[string][]string{},
	}
	positionals := []string{}
	endOfFlags := false

	for index := 0; index < len(argv); index++ {
		token := argv[index]

		if endOfFlags || !isFlag(token) {
			positionals = append(positionals, token)
			continue
		}
		if token == "--" {
			endOfFlags = true
			continue
		}

		body := token[1:]
		if strings.HasPrefix(token, "--") {
			body = token[2:]
		}
		name, inline, hasInline := strings.Cut(body, "=")
		if use, gone := retired[name]; gone {
			return ParsedArgv{}, errs.Userf("flag --%s is no longer accepted: use %s", name, use)
		}
		name = expand(name)

		if booleanFlags[name] {
			flags.bools[name] = !hasInline || inline != "false"
			continue
		}

		value := inline
		if !hasInline {
			if index+1 >= len(argv) || isFlag(argv[index+1]) {
				return ParsedArgv{}, errs.Userf("flag --%s requires a value", name)
			}
			value = argv[index+1]
			index++
		}

		if repeatable[name] {
			flags.lists[name] = append(flags.lists[name], value)
		} else {
			flags.values[name] = value
		}
	}

	parsed := ParsedArgv{Args: positionals, Flags: flags}
	if len(positionals) > 0 {
		parsed.Command = positionals[0]
		parsed.Args = positionals[1:]
	}
	return parsed, nil
}

func expand(name string) string {
	if long, ok := aliases[name]; ok {
		return long
	}
	return name
}

// isFlag reports whether a token is a flag. A lone `-` is a conventional stdin placeholder.
func isFlag(token string) bool {
	return strings.HasPrefix(token, "-") && token != "-"
}
