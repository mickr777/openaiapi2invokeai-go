package workflow

import "testing"

func TestSuggestGraphGeneration(t *testing.T) {
	graph := map[string]any{
		"nodes": map[string]any{
			"loader":  map[string]any{"id": "loader", "type": "krea2_model_loader", "model": map[string]any{"name": "Krea 2 Turbo"}},
			"text":    map[string]any{"id": "text", "type": "krea2_text_encoder", "prompt": ""},
			"denoise": map[string]any{"id": "denoise", "type": "krea2_denoise", "seed": 0, "width": 1360, "height": 768, "steps": 8, "cfg_scale": 1.0},
			"decode":  map[string]any{"id": "decode", "type": "qwen_image_l2i", "is_intermediate": false},
		},
		"edges": []any{
			map[string]any{"source": map[string]any{"node_id": "text"}, "destination": map[string]any{"node_id": "denoise"}},
			map[string]any{"source": map[string]any{"node_id": "denoise"}, "destination": map[string]any{"node_id": "decode"}},
		},
	}
	s := SuggestGraph(graph, "misleading-flux-name.json")
	if s.SuggestedModelID != "krea2" || s.WorkflowType != "generation" || s.OutputNode != "decode" {
		t.Fatalf("unexpected suggestion: %+v", s)
	}
	if s.Mapping.Prompt != "nodes.text.prompt" || s.Mapping.Width != "nodes.denoise.width" {
		t.Fatalf("unexpected mapping: %+v", s.Mapping)
	}
	if len(s.Blocking) != 0 {
		t.Fatalf("blocking: %v", s.Blocking)
	}
}

func TestSuggestGraphEditUsesImageInput(t *testing.T) {
	graph := map[string]any{
		"nodes": map[string]any{
			"image":   map[string]any{"id": "image", "type": "image", "image": map[string]any{}},
			"text":    map[string]any{"id": "text", "type": "text_encoder", "prompt": ""},
			"denoise": map[string]any{"id": "denoise", "type": "flux2_denoise", "seed": 0, "width": 768, "height": 1360, "num_steps": 4, "denoising_start": 0.3},
			"decode":  map[string]any{"id": "decode", "type": "flux2_vae_decode", "is_intermediate": false},
		},
		"edges": []any{
			map[string]any{"source": map[string]any{"node_id": "image"}, "destination": map[string]any{"node_id": "denoise"}},
			map[string]any{"source": map[string]any{"node_id": "text"}, "destination": map[string]any{"node_id": "denoise"}},
			map[string]any{"source": map[string]any{"node_id": "denoise"}, "destination": map[string]any{"node_id": "decode"}},
		},
	}
	s := SuggestGraph(graph, "flux-edit.json")
	if s.WorkflowType != "edit" || s.Mapping.Image != "nodes.image.image" || s.OutputNode != "decode" {
		t.Fatalf("unexpected edit suggestion: %+v", s)
	}
}

func TestSuggestGraphAmbiguousOutputBlocks(t *testing.T) {
	graph := map[string]any{
		"nodes": map[string]any{
			"text": map[string]any{"id": "text", "type": "text_encoder", "prompt": ""},
			"a":    map[string]any{"id": "a", "type": "vae_decode", "is_intermediate": false},
			"b":    map[string]any{"id": "b", "type": "vae_decode", "is_intermediate": false},
		},
		"edges": []any{},
	}
	s := SuggestGraph(graph, "ambiguous.json")
	if len(s.Blocking) == 0 {
		t.Fatal("expected blocking ambiguity")
	}
}
