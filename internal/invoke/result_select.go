package invoke

import (
	"fmt"
	"sort"
)

type imageCandidate struct {
	execID   string
	image    string
	sourceID string
}

// SelectImageResult deterministically chooses the final image from a completed
// InvokeAI queue item. Result map iteration order is deliberately ignored.
func SelectImageResult(detail *QueueItemDetail, graph Graph, preferredSourceID ...string) (string, string, error) {
	if detail == nil {
		return "", "", fmt.Errorf("nil queue item detail")
	}

	images := make(map[string]string)
	for execID, out := range detail.Session.Results {
		if out.Image != nil && out.Image.ImageName != "" {
			images[execID] = out.Image.ImageName
		}
	}
	if len(images) == 0 {
		return "", "", fmt.Errorf("no images in output")
	}

	preferred := ""
	if len(preferredSourceID) > 0 {
		preferred = preferredSourceID[0]
	}
	if preferred != "" {
		var matches []imageCandidate
		for execID, name := range images {
			sourceID := detail.Session.PreparedSourceMapping[execID]
			if execID == preferred || sourceID == preferred {
				matches = append(matches, imageCandidate{execID: execID, image: name, sourceID: sourceID})
			}
		}
		if len(matches) == 1 {
			return matches[0].execID, matches[0].image, nil
		}
		if len(matches) > 1 {
			return "", "", ambiguousImageError(matches, "configured output node produced multiple image results")
		}
		if graphNode(graph, preferred) == nil {
			return "", "", fmt.Errorf("configured output node %q does not exist in submitted workflow", preferred)
		}
		if len(images) == 1 && explicitlyNonIntermediate(graphNode(graph, preferred)) {
			for execID, name := range images {
				return execID, name, nil
			}
		}
		return "", "", fmt.Errorf("configured output node %q produced no identifiable image result", preferred)
	}

	if len(images) == 1 {
		for execID, name := range images {
			return execID, name, nil
		}
	}

	candidates := make([]imageCandidate, 0, len(images))
	for execID, name := range images {
		sourceID := detail.Session.PreparedSourceMapping[execID]
		if sourceID == "" && graphNode(graph, execID) != nil {
			sourceID = execID
		}
		candidates = append(candidates, imageCandidate{execID: execID, image: name, sourceID: sourceID})
	}

	var final []imageCandidate
	for _, c := range candidates {
		if node := graphNode(graph, c.sourceID); node != nil && explicitlyNonIntermediate(node) {
			final = append(final, c)
		}
	}
	if len(final) == 1 {
		return final[0].execID, final[0].image, nil
	}
	if len(final) > 1 {
		return "", "", ambiguousImageError(candidates, "multiple non-intermediate image results")
	}

	terminal := final[:0]
	for _, c := range candidates {
		if c.sourceID != "" && graphNode(graph, c.sourceID) != nil && !hasOutgoingEdge(graph, c.sourceID) {
			terminal = append(terminal, c)
		}
	}
	if len(terminal) == 1 {
		return terminal[0].execID, terminal[0].image, nil
	}

	execFinal := final[:0]
	for _, c := range candidates {
		if node := graphNode(detail.Session.ExecutionGraph, c.execID); node != nil && explicitlyNonIntermediate(node) {
			execFinal = append(execFinal, c)
		}
	}
	if len(execFinal) == 1 {
		return execFinal[0].execID, execFinal[0].image, nil
	}

	return "", "", ambiguousImageError(candidates, "could not identify a unique final image")
}

func graphNode(graph Graph, nodeID string) map[string]any {
	if nodeID == "" || graph == nil {
		return nil
	}
	nodes, ok := graph["nodes"].(map[string]any)
	if !ok {
		return nil
	}
	node, _ := nodes[nodeID].(map[string]any)
	return node
}

func explicitlyNonIntermediate(node map[string]any) bool {
	v, ok := node["is_intermediate"]
	if !ok {
		v, ok = node["isIntermediate"]
	}
	b, ok := v.(bool)
	return ok && !b
}

func hasOutgoingEdge(graph Graph, nodeID string) bool {
	edges, ok := graph["edges"].([]any)
	if !ok {
		return false
	}
	for _, raw := range edges {
		edge, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		source, ok := edge["source"].(map[string]any)
		if !ok {
			continue
		}
		if source["node_id"] == nodeID || source["nodeId"] == nodeID {
			return true
		}
	}
	return false
}

func ambiguousImageError(candidates []imageCandidate, reason string) error {
	parts := make([]string, 0, len(candidates))
	for _, c := range candidates {
		source := c.sourceID
		if source == "" {
			source = "?"
		}
		parts = append(parts, fmt.Sprintf("%s(source=%s,image=%s)", c.execID, source, c.image))
	}
	sort.Strings(parts)
	return fmt.Errorf("%s: %v", reason, parts)
}
