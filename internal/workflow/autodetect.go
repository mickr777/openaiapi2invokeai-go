package workflow

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type WorkflowSuggestion struct {
	SuggestedModelID string       `json:"suggested_model_id"`
	ModelName        string       `json:"model_name,omitempty"`
	WorkflowType     string       `json:"workflow_type"`
	OutputNode       string       `json:"output_node,omitempty"`
	Mapping          FieldMapping `json:"mapping"`
	Warnings         []string     `json:"warnings,omitempty"`
	Blocking         []string     `json:"blocking,omitempty"`
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func SuggestWorkflow(dataDir, filename string) (WorkflowSuggestion, error) {
	parsed, err := LoadWorkflow(dataDir, filename)
	if err != nil {
		return WorkflowSuggestion{}, err
	}
	return SuggestGraph(parsed.Graph, filename), nil
}

func SuggestGraph(graph map[string]any, filename string) WorkflowSuggestion {
	nodes, _ := graph["nodes"].(map[string]any)
	s := WorkflowSuggestion{WorkflowType: "generation"}
	if len(nodes) == 0 {
		s.Blocking = append(s.Blocking, "No executable nodes were found.")
		return s
	}

	outgoing := make(map[string]map[string]struct{}, len(nodes))
	for id := range nodes {
		outgoing[id] = map[string]struct{}{}
	}
	if edges, ok := graph["edges"].([]any); ok {
		for _, raw := range edges {
			edge, _ := raw.(map[string]any)
			source, _ := edge["source"].(map[string]any)
			dest, _ := edge["destination"].(map[string]any)
			src, _ := source["node_id"].(string)
			dst, _ := dest["node_id"].(string)
			if _, ok := nodes[src]; ok {
				if _, ok := nodes[dst]; ok {
					outgoing[src][dst] = struct{}{}
				}
			}
		}
	}

	reaches := func(src, dst string) bool {
		pending := []string{src}
		seen := map[string]struct{}{}
		for len(pending) > 0 {
			cur := pending[0]
			pending = pending[1:]
			if cur == dst {
				return true
			}
			if _, ok := seen[cur]; ok {
				continue
			}
			seen[cur] = struct{}{}
			for next := range outgoing[cur] {
				if _, ok := seen[next]; !ok {
					pending = append(pending, next)
				}
			}
		}
		return false
	}

	var outputs []string
	for id, raw := range nodes {
		node, _ := raw.(map[string]any)
		if isSuggestedOutputType(strVal(node, "type")) && len(outgoing[id]) == 0 {
			outputs = append(outputs, id)
		}
	}
	if len(outputs) == 0 {
		for id, raw := range nodes {
			node, _ := raw.(map[string]any)
			if isSuggestedOutputType(strVal(node, "type")) {
				outputs = append(outputs, id)
			}
		}
	}

	var nonIntermediate []string
	for _, id := range outputs {
		node, _ := nodes[id].(map[string]any)
		if v, ok := node["is_intermediate"].(bool); !ok || !v {
			nonIntermediate = append(nonIntermediate, id)
		}
	}
	switch {
	case len(nonIntermediate) == 1:
		s.OutputNode = nonIntermediate[0]
	case len(outputs) == 1:
		s.OutputNode = outputs[0]
		s.Warnings = append(s.Warnings, "Final image node is marked intermediate; the proxy will still use the selected output node.")
	default:
		s.Blocking = append(s.Blocking, "Cannot choose one final image node; select its exact node ID manually.")
	}

	var imageCandidates []string
	for id, raw := range nodes {
		node, _ := raw.(map[string]any)
		if isImageInputType(strVal(node, "type")) {
			if s.OutputNode == "" || reaches(id, s.OutputNode) {
				imageCandidates = append(imageCandidates, id)
			}
		}
	}
	preferredImages := filterIDs(imageCandidates, func(id string) bool {
		node, _ := nodes[id].(map[string]any)
		return strVal(node, "type") == "image"
	})
	imageNode := ""
	switch {
	case len(preferredImages) == 1:
		imageNode = preferredImages[0]
	case len(imageCandidates) == 1:
		imageNode = imageCandidates[0]
	case len(imageCandidates) > 1:
		s.Warnings = append(s.Warnings, "Multiple possible image inputs were found; map the correct input manually.")
	}
	if len(imageCandidates) > 0 {
		s.WorkflowType = "edit"
	}

	var modelName, loaderType string
	var loaders []map[string]any
	for _, raw := range nodes {
		node, _ := raw.(map[string]any)
		if strings.Contains(strings.ToLower(strVal(node, "type")), "model_loader") {
			loaders = append(loaders, node)
		}
	}
	if len(loaders) == 1 {
		loaderType = strVal(loaders[0], "type")
		if model, ok := loaders[0]["model"].(map[string]any); ok {
			modelName, _ = model["name"].(string)
		}
	} else if len(loaders) > 1 {
		s.Warnings = append(s.Warnings, "Several model loaders were found; confirm the model identity before saving.")
	} else {
		s.Warnings = append(s.Warnings, "No model loader was found; the model ID is derived from the workflow filename.")
	}
	s.ModelName = modelName
	s.SuggestedModelID = modelSlug(modelName, loaderType, filename)

	choose := func(label, field string, eligible func(map[string]any) bool) string {
		var ids []string
		for id, raw := range nodes {
			node, _ := raw.(map[string]any)
			if !eligible(node) {
				continue
			}
			if _, ok := node[field]; !ok {
				continue
			}
			ids = append(ids, id)
		}
		if s.OutputNode != "" {
			var connected []string
			for _, id := range ids {
				if reaches(id, s.OutputNode) {
					connected = append(connected, id)
				}
			}
			if len(connected) > 0 {
				ids = connected
			}
		}
		if len(ids) == 1 {
			return fmt.Sprintf("nodes.%s.%s", ids[0], field)
		}
		if len(ids) > 1 {
			s.Warnings = append(s.Warnings, fmt.Sprintf("Multiple %s fields were detected; configure %s manually.", label, label))
		}
		return ""
	}

	isDenoise := func(node map[string]any) bool {
		return strings.Contains(strings.ToLower(strVal(node, "type")), "denoise")
	}
	isText := func(node map[string]any) bool {
		kind := strings.ToLower(strVal(node, "type"))
		return strings.Contains(kind, "text_encoder") || kind == "string" || kind == "string_input"
	}

	s.Mapping.Prompt = choose("prompt", "prompt", isText)
	if s.Mapping.Prompt == "" {
		s.Mapping.Prompt = choose("prompt", "value", func(node map[string]any) bool {
			kind := strings.ToLower(strVal(node, "type"))
			return kind == "string" || kind == "string_input"
		})
	}
	s.Mapping.Seed = chooseFirst("seed", []string{"seed"}, choose, isDenoise)
	s.Mapping.Width = chooseFirst("width", []string{"width"}, choose, isDenoise)
	s.Mapping.Height = chooseFirst("height", []string{"height"}, choose, isDenoise)
	s.Mapping.Steps = chooseFirst("steps", []string{"steps", "num_steps"}, choose, isDenoise)
	cfgFields := []string{"cfg_scale", "guidance"}
	if strings.Contains(strings.ToLower(loaderType), "flux2") {
		cfgFields = []string{"guidance", "cfg_scale"}
	}
	s.Mapping.CFG = chooseFirst("cfg", cfgFields, choose, isDenoise)
	s.Mapping.Denoise = chooseFirst("denoise", []string{"denoising_start"}, choose, isDenoise)
	if imageNode != "" {
		s.Mapping.Image = fmt.Sprintf("nodes.%s.image", imageNode)
	}

	if s.Mapping.Prompt == "" {
		s.Blocking = append(s.Blocking, "A prompt field could not be chosen unambiguously.")
	}
	if s.OutputNode == "" && !containsString(s.Blocking, "Cannot choose one final image node; select its exact node ID manually.") {
		s.Blocking = append(s.Blocking, "An exact final output node is required.")
	}
	if s.WorkflowType == "edit" && imageNode == "" {
		s.Blocking = append(s.Blocking, "An edit image input could not be chosen unambiguously.")
	}
	s.Warnings = uniqueStrings(s.Warnings)
	s.Blocking = uniqueStrings(s.Blocking)
	return s
}

func chooseFirst(label string, fields []string, choose func(string, string, func(map[string]any) bool) string, eligible func(map[string]any) bool) string {
	for _, field := range fields {
		if path := choose(label, field, eligible); path != "" {
			return path
		}
	}
	return ""
}

func isSuggestedOutputType(kind string) bool {
	kind = strings.ToLower(kind)
	return kind == "qwen_image_l2i" || kind == "flux2_vae_decode" || kind == "l2i" ||
		kind == "vae_decode" || kind == "save_image" || strings.HasSuffix(kind, "_l2i") ||
		strings.HasSuffix(kind, "_vae_decode")
}

func isImageInputType(kind string) bool {
	switch strings.ToLower(kind) {
	case "image", "qwen_image_i2l", "flux_kontext":
		return true
	default:
		return false
	}
}

func modelSlug(modelName, loaderType, filename string) string {
	name := strings.ToLower(modelName + " " + loaderType)
	switch {
	case strings.Contains(name, "krea"):
		return "krea2"
	case strings.Contains(name, "flux") && strings.Contains(name, "klein"):
		return "flux2klein"
	case strings.Contains(name, "qwen") && strings.Contains(name, "image"):
		return "qwenimage"
	case strings.Contains(name, "z-image") || strings.Contains(name, "z_image"):
		return "zimage"
	}
	candidate := modelName
	if candidate == "" {
		candidate = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	candidate = slugPattern.ReplaceAllString(strings.ToLower(candidate), "-")
	candidate = strings.Trim(candidate, "-")
	if len(candidate) > 55 {
		candidate = candidate[:55]
	}
	if candidate == "" {
		return "imported-model"
	}
	return candidate
}

func filterIDs(ids []string, keep func(string) bool) []string {
	var out []string
	for _, id := range ids {
		if keep(id) {
			out = append(out, id)
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
