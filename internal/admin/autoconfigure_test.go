package admin

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pfannkuchensack/openaiapi2invokeai-go/internal/config"
	"github.com/Pfannkuchensack/openaiapi2invokeai-go/internal/workflow"
)

func newAutoConfigTestHandler(t *testing.T) (*Handler, *workflow.Registry, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	reg, err := workflow.NewRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(&config.Config{DataDir: dir}, reg, nil, log), reg, dir
}

func writeAutoConfigWorkflow(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "workflows", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func postAutoConfig(t *testing.T, h *Handler, name string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/workflows/auto-configure/"+name, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	return rec
}

func TestWorkflowAutoConfigureGenerationThenEdit(t *testing.T) {
	h, reg, dir := newAutoConfigTestHandler(t)
	writeAutoConfigWorkflow(t, dir, "krea-generation.json", `{
		"nodes": {
			"loader": {"id":"loader","type":"krea2_model_loader","model":{"name":"Krea 2 Turbo"}},
			"text": {"id":"text","type":"krea2_text_encoder","prompt":""},
			"denoise": {"id":"denoise","type":"krea2_denoise","seed":0,"width":1360,"height":768,"steps":8,"cfg_scale":1},
			"decode": {"id":"decode","type":"qwen_image_l2i","is_intermediate":false}
		},
		"edges": [
			{"source":{"node_id":"text","field":"conditioning"},"destination":{"node_id":"denoise","field":"conditioning"}},
			{"source":{"node_id":"denoise","field":"latents"},"destination":{"node_id":"decode","field":"latents"}}
		]
	}`)
	rec := postAutoConfig(t, h, "krea-generation.json", url.Values{
		"confirm": {"yes"}, "model_id": {"krea2"}, "role": {"generation"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("generation status=%d body=%s", rec.Code, rec.Body.String())
	}
	entry, ok := reg.Get("krea2")
	if !ok {
		t.Fatal("model was not created")
	}
	if entry.Workflow != "krea-generation.json" || entry.OutputNode != "decode" {
		t.Fatalf("generation entry=%+v", entry)
	}
	if entry.MappingFor("generation").Prompt != "nodes.text.prompt" {
		t.Fatalf("generation mapping=%+v", entry.MappingFor("generation"))
	}

	writeAutoConfigWorkflow(t, dir, "krea-edit.json", `{
		"nodes": {
			"image": {"id":"image","type":"image","image":{"image_name":"placeholder.png"}},
			"text": {"id":"text","type":"krea2_text_encoder","prompt":""},
			"denoise": {"id":"denoise","type":"krea2_denoise","seed":0,"width":768,"height":1360,"steps":8,"cfg_scale":1,"denoising_start":0.35},
			"decode": {"id":"decode","type":"qwen_image_l2i","is_intermediate":false}
		},
		"edges": [
			{"source":{"node_id":"image","field":"image"},"destination":{"node_id":"denoise","field":"image"}},
			{"source":{"node_id":"text","field":"conditioning"},"destination":{"node_id":"denoise","field":"conditioning"}},
			{"source":{"node_id":"denoise","field":"latents"},"destination":{"node_id":"decode","field":"latents"}}
		]
	}`)
	rec = postAutoConfig(t, h, "krea-edit.json", url.Values{
		"confirm": {"yes"}, "model_id": {"krea2"}, "role": {"edit"}, "also_variation": {"yes"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("edit status=%d body=%s", rec.Code, rec.Body.String())
	}
	entry, _ = reg.Get("krea2")
	if entry.Workflow != "krea-generation.json" {
		t.Fatalf("generation workflow was overwritten: %+v", entry)
	}
	if entry.EditWorkflow != "krea-edit.json" || entry.VariantWorkflow != "krea-edit.json" {
		t.Fatalf("edit assignment=%+v", entry)
	}
	if entry.MappingFor("edit").Image != "nodes.image.image" || entry.EditOutputNode != "decode" {
		t.Fatalf("edit mapping=%+v output=%s", entry.MappingFor("edit"), entry.EditOutputNode)
	}
}

func TestWorkflowAutoConfigureRequiresGenerationBeforeEdit(t *testing.T) {
	h, _, dir := newAutoConfigTestHandler(t)
	writeAutoConfigWorkflow(t, dir, "edit.json", `{
		"nodes": {
			"image": {"id":"image","type":"image","image":{}},
			"text": {"id":"text","type":"text_encoder","prompt":""},
			"decode": {"id":"decode","type":"vae_decode","is_intermediate":false}
		},
		"edges": [
			{"source":{"node_id":"image"},"destination":{"node_id":"decode"}},
			{"source":{"node_id":"text"},"destination":{"node_id":"decode"}}
		]
	}`)
	rec := postAutoConfig(t, h, "edit.json", url.Values{
		"confirm": {"yes"}, "model_id": {"missing"}, "role": {"edit"},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWorkflowAutoConfigureDoesNotReplaceGenerationWithoutConfirmation(t *testing.T) {
	h, reg, dir := newAutoConfigTestHandler(t)
	if err := reg.Put(workflow.ModelEntry{ID: "krea2", Workflow: "old.json"}); err != nil {
		t.Fatal(err)
	}
	writeAutoConfigWorkflow(t, dir, "new.json", `{
		"nodes": {
			"text": {"id":"text","type":"text_encoder","prompt":""},
			"decode": {"id":"decode","type":"vae_decode","is_intermediate":false}
		},
		"edges": [{"source":{"node_id":"text"},"destination":{"node_id":"decode"}}]
	}`)
	rec := postAutoConfig(t, h, "new.json", url.Values{
		"confirm": {"yes"}, "model_id": {"krea2"}, "role": {"generation"},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	entry, _ := reg.Get("krea2")
	if entry.Workflow != "old.json" {
		t.Fatalf("existing generation workflow changed: %+v", entry)
	}
}
