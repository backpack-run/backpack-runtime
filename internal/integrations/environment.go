package integrations

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"
)

const redacted = "[REDACTED]"

type Variable struct {
	name      string
	value     string
	sensitive bool
	remove    bool
}

func Set(name, value string) (Variable, error)    { return newVariable(name, value, false, false) }
func Secret(name, value string) (Variable, error) { return newVariable(name, value, true, false) }
func Unset(name string) (Variable, error)         { return newVariable(name, "", false, true) }
func (v Variable) Name() string                   { return v.name }
func (v Variable) Sensitive() bool                { return v.sensitive }
func (v Variable) Removed() bool                  { return v.remove }
func (v Variable) String() string                 { return v.redactedEntry() }
func (v Variable) Format(state fmt.State, _ rune) { _, _ = io.WriteString(state, v.redactedEntry()) }
func (v Variable) redactedEntry() string {
	if v.remove {
		return v.name + "=<unset>"
	}
	if v.sensitive {
		return v.name + "=" + redacted
	}
	return v.name + "=" + v.value
}

func newVariable(name, value string, sensitive, remove bool) (Variable, error) {
	if name == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, "=\x00") {
		return Variable{}, fmt.Errorf("invalid environment variable name %q", name)
	}
	if strings.ContainsRune(value, '\x00') {
		return Variable{}, fmt.Errorf("environment variable %q contains a NUL byte", name)
	}
	return Variable{name: name, value: value, sensitive: sensitive, remove: remove}, nil
}

// EnvironmentOverlay keeps values private and provides an explicitly redacted
// representation for diagnostics. Apply is the only operation that reveals
// values, directly into the child-process environment representation.
type EnvironmentOverlay struct{ variables []Variable }

func NewEnvironmentOverlay(variables ...Variable) (EnvironmentOverlay, error) {
	seen := map[string]struct{}{}
	copyVariables := make([]Variable, 0, len(variables))
	for _, variable := range variables {
		if variable.name == "" {
			return EnvironmentOverlay{}, fmt.Errorf("environment overlay contains an uninitialized variable")
		}
		key := environmentKey(variable.name)
		if _, ok := seen[key]; ok {
			return EnvironmentOverlay{}, fmt.Errorf("environment variable %q is repeated in overlay", variable.name)
		}
		seen[key] = struct{}{}
		copyVariables = append(copyVariables, variable)
	}
	return EnvironmentOverlay{variables: copyVariables}, nil
}

func (o EnvironmentOverlay) Apply(base []string) []string {
	overrides := make(map[string]Variable, len(o.variables))
	for _, variable := range o.variables {
		overrides[environmentKey(variable.name)] = variable
	}
	result := make([]string, 0, len(base)+len(o.variables))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, replaced := overrides[environmentKey(name)]; !replaced {
			result = append(result, entry)
		}
	}
	for _, variable := range o.variables {
		if !variable.remove {
			result = append(result, variable.name+"="+variable.value)
		}
	}
	return result
}

func (o EnvironmentOverlay) Redacted() []string {
	result := make([]string, 0, len(o.variables))
	for _, variable := range o.variables {
		result = append(result, variable.redactedEntry())
	}
	sort.Strings(result)
	return result
}

func (o EnvironmentOverlay) String() string { return strings.Join(o.Redacted(), " ") }
func (o EnvironmentOverlay) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, o.String())
}

func environmentKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}
