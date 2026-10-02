package invoke

import "testing"

func TestSelectImageResultHonorsConfiguredSourceNode(t *testing.T) {
	g := Graph{"nodes": map[string]any{
		"input":  map[string]any{"id": "input", "type": "image", "is_intermediate": false},
		"decode": map[string]any{"id": "decode", "type": "decode", "is_intermediate": false},
	}}
	d := detailWithImages(
		map[string]string{"uuid-input": "original.png", "uuid-final": "edited.png"},
		map[string]string{"uuid-input": "input", "uuid-final": "decode"},
	)
	_, name, err := SelectImageResult(d, g, "decode")
	if err != nil {
		t.Fatal(err)
	}
	if name != "edited.png" {
		t.Fatalf("got %q", name)
	}
}

func TestSelectImageResultRejectsWrongConfiguredNode(t *testing.T) {
	g := Graph{"nodes": map[string]any{
		"decode": map[string]any{"id": "decode", "type": "decode", "is_intermediate": false},
	}}
	d := detailWithImages(
		map[string]string{"a": "one.png", "b": "two.png"},
		map[string]string{"a": "x", "b": "y"},
	)
	if _, _, err := SelectImageResult(d, g, "decode"); err == nil {
		t.Fatal("expected configured output mismatch to fail")
	}
}
