package workflow

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
)

// Params are the OpenAI-facing parameters to inject into a workflow graph.
type Params struct {
	Prompt   string
	Negative string
	Width    int
	Height   int
	Seed     int64 // -1 or 0 means random
	Steps    int
	CFG      float64
}

// BuildGraph loads a workflow file and applies parameter substitution based on
// the model entry's field mapping. Returns the ready-to-enqueue graph.
func BuildGraph(dataDir string, entry ModelEntry, params Params) (map[string]any, error) {
	return buildGraph(dataDir, entry.Workflow, entry, params, entry.MappingFor("generation"))
}

// BuildGraphFromFile preserves the historical behavior of using the legacy
// mapping field. New endpoint-specific callers should use BuildGraphForRole.
func BuildGraphFromFile(dataDir string, workflowFile string, entry ModelEntry, params Params) (map[string]any, error) {
	return buildGraph(dataDir, workflowFile, entry, params, entry.Mapping)
}

func BuildGraphForRole(dataDir string, workflowFile string, entry ModelEntry, params Params, role string) (map[string]any, error) {
	return buildGraph(dataDir, workflowFile, entry, params, entry.MappingFor(role))
}

func buildGraph(dataDir, workflowFile string, entry ModelEntry, params Params, mapping FieldMapping) (map[string]any, error) {
	parsed, err := LoadWorkflow(dataDir, workflowFile)
	if err != nil {
		return nil, err
	}
	graph := parsed.Graph

	if entry.Defaults != nil {
		for key, val := range entry.Defaults {
			path := mappingPathForKey(mapping, key)
			if path != "" {
				setField(graph, path, val)
			}
		}
	}

	if params.Prompt != "" && mapping.Prompt != "" {
		setField(graph, mapping.Prompt, params.Prompt)
	}
	if params.Negative != "" && mapping.Negative != "" {
		setField(graph, mapping.Negative, params.Negative)
	}
	if params.Width > 0 && mapping.Width != "" {
		setField(graph, mapping.Width, params.Width)
	}
	if params.Height > 0 && mapping.Height != "" {
		setField(graph, mapping.Height, params.Height)
	}
	if mapping.Seed != "" {
		seed := params.Seed
		if seed <= 0 {
			seed = rand.Int64N(2147483647)
		}
		setField(graph, mapping.Seed, seed)
	}
	if params.Steps > 0 && mapping.Steps != "" {
		setField(graph, mapping.Steps, params.Steps)
	}
	if params.CFG > 0 && mapping.CFG != "" {
		setField(graph, mapping.CFG, params.CFG)
	}

	return graph, nil
}

// setField sets a value at a dot-separated path in a nested map.
// Path format: "nodes.<node-id>.<field>" e.g. "nodes.abc123.value"
func setField(graph map[string]any, path string, value any) {
	parts := splitPath(path)
	if len(parts) < 2 {
		return
	}

	current := any(graph)
	for i := 0; i < len(parts)-1; i++ {
		switch m := current.(type) {
		case map[string]any:
			next, ok := m[parts[i]]
			if !ok {
				return
			}
			current = next
		default:
			return
		}
	}

	if m, ok := current.(map[string]any); ok {
		m[parts[len(parts)-1]] = value
	}
}

// splitPath splits a dot-path but preserves dots inside node UUIDs.
// Format: "nodes.<uuid>.field" → ["nodes", "<uuid>", "field"]
func splitPath(path string) []string {
	// Split on dots, but reassemble UUID parts (contains dashes, not dots)
	return strings.Split(path, ".")
}

// mappingPathForKey maps a defaults key name to a field mapping path.
func mappingPathForKey(m FieldMapping, key string) string {
	switch key {
	case "steps":
		return m.Steps
	case "cfg":
		return m.CFG
	case "seed":
		return m.Seed
	case "width":
		return m.Width
	case "height":
		return m.Height
	case "prompt":
		return m.Prompt
	case "negative":
		return m.Negative
	}
	return ""
}

// SetGraphField sets a value at a dot-path in a graph. Exported for use by handlers.
func SetGraphField(graph map[string]any, path string, value any) {
	setField(graph, path, value)
}

// ResolveSize resolves an OpenAI size string (e.g. "1024x1024") to width/height
// using the model's size presets, or parses directly.
func ResolveSize(entry ModelEntry, sizeStr string) (int, int, error) {
	if sizeStr == "" {
		return 0, 0, nil
	}

	// Check presets first, so a model can define its own "auto" if it wants to
	if preset, ok := entry.SizePresets[sizeStr]; ok {
		return preset.Width, preset.Height, nil
	}

	// OpenAI's own default for gpt-image-1 is "auto", and clients send it
	// verbatim. It means "you decide", so leave the workflow's size alone.
	if strings.EqualFold(sizeStr, "auto") {
		return 0, 0, nil
	}

	// Try to parse "WxH"
	var w, h int
	if _, err := fmt.Sscanf(sizeStr, "%dx%d", &w, &h); err == nil && w > 0 && h > 0 {
		return w, h, nil
	}

	return 0, 0, fmt.Errorf("unknown size %q (not in presets and not WxH format)", sizeStr)
}

var promptSizePattern = regexp.MustCompile(`(?i)([0-9]{2,5})[[:space:]]*(x|×|by)[[:space:]]*([0-9]{2,5})`)

// ResolveGenerationSize behaves like ResolveSize, but when a client explicitly
// requests "auto" it can infer a useful size from the prompt. This mirrors the
// intent of GPT Image's auto mode while keeping workflow defaults when the
// prompt contains no size or orientation hint.
func ResolveGenerationSize(entry ModelEntry, sizeStr, prompt string) (int, int, error) {
	w, h, err := ResolveSize(entry, sizeStr)
	if err != nil {
		return 0, 0, err
	}
	if w > 0 && h > 0 {
		return w, h, nil
	}
	trimmedSize := strings.TrimSpace(sizeStr)
	if trimmedSize != "" && !strings.EqualFold(trimmedSize, "auto") {
		return w, h, nil
	}
	if inferredW, inferredH, ok := InferPromptSize(prompt); ok {
		return inferredW, inferredH, nil
	}
	return 0, 0, nil
}

// InferPromptSize extracts an explicit WIDTHxHEIGHT request, or falls back to
// conservative orientation keywords. Values outside a sensible image range
// are ignored instead of overriding the workflow.
func InferPromptSize(prompt string) (int, int, bool) {
	if match := promptSizePattern.FindStringSubmatch(prompt); len(match) == 4 {
		w, errW := strconv.Atoi(match[1])
		h, errH := strconv.Atoi(match[3])
		if errW == nil && errH == nil && w >= 64 && h >= 64 && w <= 8192 && h <= 8192 {
			return w, h, true
		}
	}

	p := strings.ToLower(prompt)
	containsAny := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(p, value) {
				return true
			}
		}
		return false
	}

	switch {
	case containsAny("square", "1:1", "1 / 1", "1 to 1"):
		return 1024, 1024, true
	case containsAny("portrait", "vertical", "phone wallpaper", "mobile wallpaper", "story format", "9:16"):
		return 768, 1360, true
	case containsAny("landscape", "wide", "cinematic", "banner", "16:9", "21:9", "desktop wallpaper"):
		return 1360, 768, true
	default:
		return 0, 0, false
	}
}

