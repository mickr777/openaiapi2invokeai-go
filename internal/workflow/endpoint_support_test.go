package workflow

import "testing"

func TestEndpointSupportUsesRoleSpecificMappings(t *testing.T) {
	entry := ModelEntry{
		Workflow:        "generation.json",
		EditWorkflow:    "edit.json",
		VariantWorkflow: "edit.json",
		EditMapping: FieldMapping{
			Image: "nodes.i2l.image",
		},
	}
	if !entry.SupportsEdit() {
		t.Fatal("edit should be supported by edit-specific image mapping")
	}
	if !entry.SupportsVariation() {
		t.Fatal("variation should inherit edit image mapping")
	}
}

func TestEndpointSupportRequiresWorkflowAndImageMapping(t *testing.T) {
	entry := ModelEntry{EditWorkflow: "edit.json"}
	if entry.SupportsEdit() {
		t.Fatal("edit should require an image mapping")
	}
	entry = ModelEntry{EditMapping: FieldMapping{Image: "nodes.i2l.image"}}
	if entry.SupportsEdit() {
		t.Fatal("edit should require an edit workflow")
	}
}
