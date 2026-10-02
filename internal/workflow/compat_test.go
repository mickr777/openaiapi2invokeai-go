package workflow

import "testing"

func TestInferPromptSize(t *testing.T) {
	tests := []struct {
		prompt string
		w, h   int
		ok     bool
	}{
		{"make this 768x1360 please", 768, 1360, true},
		{"render at 1344 by 768 cinematic", 1344, 768, true},
		{"use 1024×1536 portrait", 1024, 1536, true},
		{"make it portrait like a phone wallpaper", 768, 1360, true},
		{"make it a wide cinematic landscape", 1360, 768, true},
		{"make it square", 1024, 1024, true},
		{"no size preference", 0, 0, false},
	}
	for _, tt := range tests {
		w, h, ok := InferPromptSize(tt.prompt)
		if w != tt.w || h != tt.h || ok != tt.ok {
			t.Fatalf("InferPromptSize(%q) = %dx%d %v, want %dx%d %v", tt.prompt, w, h, ok, tt.w, tt.h, tt.ok)
		}
	}
}

func TestResolveGenerationSizeOmittedUsesPromptHint(t *testing.T) {
	w, h, err := ResolveGenerationSize(ModelEntry{}, "", "make it portrait")
	if err != nil {
		t.Fatal(err)
	}
	if w != 768 || h != 1360 {
		t.Fatalf("got %dx%d", w, h)
	}
}

func TestResolveGenerationSizeFixedSizeWins(t *testing.T) {
	w, h, err := ResolveGenerationSize(ModelEntry{}, "1024x1024", "make it portrait")
	if err != nil {
		t.Fatal(err)
	}
	if w != 1024 || h != 1024 {
		t.Fatalf("got %dx%d", w, h)
	}
}

func TestBoardAutoSanitizedFromGraphWorkflow(t *testing.T) {
	parsed, err := ParseWorkflow([]byte(`{"nodes":{"save":{"id":"save","type":"save_image","board":"auto"}},"edges":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	node := parsed.Graph["nodes"].(map[string]any)["save"].(map[string]any)
	if _, exists := node["board"]; exists {
		t.Fatal("board=auto should be removed from API graph")
	}
}

func TestBoardAutoSanitizedFromEditorWorkflow(t *testing.T) {
	doc := []byte(`{"nodes":[{"id":"save","type":"invocation","data":{"type":"save_image","inputs":{"board":{"value":"auto"}}}}],"edges":[]}`)
	parsed, err := ParseWorkflow(doc)
	if err != nil {
		t.Fatal(err)
	}
	node := parsed.Graph["nodes"].(map[string]any)["save"].(map[string]any)
	if _, exists := node["board"]; exists {
		t.Fatal("board=auto should be removed from editor workflow")
	}
}
