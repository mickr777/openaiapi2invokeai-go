package workflow

import "testing"

func TestMappingForRoleFallbacks(t *testing.T) {
	legacy := FieldMapping{Prompt: "legacy"}
	edit := FieldMapping{Prompt: "edit"}
	entry := ModelEntry{Mapping: legacy, EditMapping: edit, EditOutputNode: "edit-out"}

	if got := entry.MappingFor("generation").Prompt; got != "legacy" {
		t.Fatalf("generation mapping = %q", got)
	}
	if got := entry.MappingFor("edit").Prompt; got != "edit" {
		t.Fatalf("edit mapping = %q", got)
	}
	if got := entry.MappingFor("variant").Prompt; got != "edit" {
		t.Fatalf("variant mapping = %q", got)
	}
	if got := entry.OutputNodeFor("variant"); got != "edit-out" {
		t.Fatalf("variant output = %q", got)
	}
}
