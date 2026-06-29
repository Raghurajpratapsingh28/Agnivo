package sbom

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

// Generator produces SPDX-style SBOM documents for built images.
type Generator struct{}

// NewGenerator constructs an SBOM generator.
func NewGenerator() *Generator { return &Generator{} }

// Generate attempts to produce an SBOM for the given image tag.
func (g *Generator) Generate(imageTag string) json.RawMessage {
	doc := map[string]any{
		"spdxVersion": "SPDX-2.3",
		"dataLicense": "CC0-1.0",
		"name":        imageTag,
		"packages":    []any{},
	}
	if out, err := exec.Command("syft", imageTag, "-o", "json").Output(); err == nil {
		return out
	}
	if out, err := exec.Command("docker", "sbom", imageTag).Output(); err == nil {
		return out
	}
	raw, _ := json.Marshal(doc)
	return raw
}

// SigningHook returns metadata for image signing (Cosign integration point).
func SigningHook(imageRef, digest string) json.RawMessage {
	meta := map[string]any{
		"signing":   "cosign",
		"image_ref": imageRef,
		"digest":    digest,
		"status":    "pending",
	}
	raw, _ := json.Marshal(meta)
	return raw
}

// WriteToFile persists SBOM to workspace for archival.
func WriteToFile(workspaceDir string, sbom json.RawMessage) error {
	if len(sbom) == 0 {
		return nil
	}
	return os.WriteFile(filepath.Join(workspaceDir, "sbom.json"), sbom, 0o644)
}
