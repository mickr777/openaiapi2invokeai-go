package invoke

import "testing"

func detailWithImages(images map[string]string, prepared map[string]string) *QueueItemDetail {
	d := &QueueItemDetail{}
	d.Session.Results = map[string]InvocationOutput{}
	d.Session.PreparedSourceMapping = prepared
	for id, name := range images {
		d.Session.Results[id] = InvocationOutput{Image: &ImageField{ImageName: name}}
	}
	return d
}

func TestSelectImageResultPreparedMapping(t *testing.T) {
	g := Graph{"nodes": map[string]any{
		"source": map[string]any{"id": "source", "type": "image", "is_intermediate": true},
		"decode": map[string]any{"id": "decode", "type": "decode", "is_intermediate": false},
	}, "edges": []any{map[string]any{"source": map[string]any{"node_id": "source"}, "destination": map[string]any{"node_id": "decode"}}}}
	d := detailWithImages(map[string]string{"uuid-original": "original.png", "uuid-final": "final.png"}, map[string]string{"uuid-original": "source", "uuid-final": "decode"})
	id, name, err := SelectImageResult(d, g)
	if err != nil {
		t.Fatal(err)
	}
	if id != "uuid-final" || name != "final.png" {
		t.Fatalf("got %s %s", id, name)
	}
}

func TestSelectImageResultTerminalFallback(t *testing.T) {
	g := Graph{"nodes": map[string]any{
		"source": map[string]any{"id": "source", "type": "image"},
		"decode": map[string]any{"id": "decode", "type": "decode", "is_intermediate": true},
	}, "edges": []any{map[string]any{"source": map[string]any{"node_id": "source"}, "destination": map[string]any{"node_id": "decode"}}}}
	d := detailWithImages(map[string]string{"a": "original.png", "b": "final.png"}, map[string]string{"a": "source", "b": "decode"})
	_, name, err := SelectImageResult(d, g)
	if err != nil {
		t.Fatal(err)
	}
	if name != "final.png" {
		t.Fatalf("got %s", name)
	}
}

func TestSelectImageResultAmbiguousFails(t *testing.T) {
	g := Graph{"nodes": map[string]any{
		"a": map[string]any{"id": "a", "type": "decode", "is_intermediate": false},
		"b": map[string]any{"id": "b", "type": "decode", "is_intermediate": false},
	}}
	d := detailWithImages(map[string]string{"x": "a.png", "y": "b.png"}, map[string]string{"x": "a", "y": "b"})
	if _, _, err := SelectImageResult(d, g); err == nil {
		t.Fatal("expected ambiguity error")
	}
}

func TestSelectImageResultSingleImage(t *testing.T) {
	d := detailWithImages(map[string]string{"x": "only.png"}, nil)
	_, name, err := SelectImageResult(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if name != "only.png" {
		t.Fatalf("got %s", name)
	}
}
