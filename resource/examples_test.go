package resource_test

import (
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/resource"
)

func TestLoadAll_ExamplesProjects(t *testing.T) {
	projects := []string{"blog", "todo", "crm"}

	for _, prj := range projects {
		t.Run("Project_"+prj, func(t *testing.T) {
			dir := filepath.Join("..", "examples", prj)
			reg, err := resource.LoadAll(dir)
			if err != nil {
				t.Fatalf("failed to LoadAll for example project '%s': %v", prj, err)
			}

			resources := reg.List()
			if len(resources) == 0 {
				t.Errorf("expected loaded resources for project '%s', got 0", prj)
			}
		})
	}
}
