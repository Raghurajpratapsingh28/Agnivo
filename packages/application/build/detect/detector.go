package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Framework identifies a detected application stack.
type Framework struct {
	Name    string
	Runtime string
	Version string
	Source  string // file that triggered detection
}

// Detector inspects a workspace and returns the best-matching framework.
type Detector struct {
	detectors []func(string) (Framework, bool)
}

// NewDetector constructs the default detector chain.
func NewDetector() *Detector {
	d := &Detector{}
	d.detectors = []func(string) (Framework, bool){
		detectDockerfile,
		detectNode,
		detectPython,
		detectGo,
		detectRust,
		detectJava,
		detectRuby,
		detectPHP,
		detectStatic,
	}
	return d
}

// Detect returns the highest-priority framework found in workspaceDir.
func (d *Detector) Detect(workspaceDir string) Framework {
	for _, fn := range d.detectors {
		if fw, ok := fn(workspaceDir); ok {
			return fw
		}
	}
	return Framework{Name: "unknown", Runtime: "generic", Source: "default"}
}

func detectDockerfile(dir string) (Framework, bool) {
	if fileExists(filepath.Join(dir, "Dockerfile")) {
		return Framework{Name: "dockerfile", Runtime: "docker", Source: "Dockerfile"}, true
	}
	return Framework{}, false
}

func detectNode(dir string) (Framework, bool) {
	path := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Framework{}, false
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return Framework{}, false
	}
	deps := mergeMaps(pkg.Dependencies, pkg.DevDependencies)
	runtime := "node20"
	switch {
	case hasDep(deps, "next"):
		return Framework{Name: "nextjs", Runtime: runtime, Source: "package.json"}, true
	case hasDep(deps, "@remix-run/react"):
		return Framework{Name: "remix", Runtime: runtime, Source: "package.json"}, true
	case hasDep(deps, "react"):
		return Framework{Name: "react", Runtime: runtime, Source: "package.json"}, true
	case hasDep(deps, "@nestjs/core"):
		return Framework{Name: "nestjs", Runtime: runtime, Source: "package.json"}, true
	case hasDep(deps, "fastify"):
		return Framework{Name: "fastify", Runtime: runtime, Source: "package.json"}, true
	case hasDep(deps, "express"):
		return Framework{Name: "express", Runtime: runtime, Source: "package.json"}, true
	default:
		return Framework{Name: "nodejs", Runtime: runtime, Source: "package.json"}, true
	}
}

func detectPython(dir string) (Framework, bool) {
	if fileExists(filepath.Join(dir, "pyproject.toml")) {
		data, _ := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
		content := string(data)
		switch {
		case strings.Contains(content, "fastapi"):
			return Framework{Name: "fastapi", Runtime: "python3.12", Source: "pyproject.toml"}, true
		case strings.Contains(content, "django"):
			return Framework{Name: "django", Runtime: "python3.12", Source: "pyproject.toml"}, true
		case strings.Contains(content, "flask"):
			return Framework{Name: "flask", Runtime: "python3.12", Source: "pyproject.toml"}, true
		}
		return Framework{Name: "python", Runtime: "python3.12", Source: "pyproject.toml"}, true
	}
	if fileExists(filepath.Join(dir, "requirements.txt")) {
		data, _ := os.ReadFile(filepath.Join(dir, "requirements.txt"))
		content := strings.ToLower(string(data))
		switch {
		case strings.Contains(content, "fastapi"):
			return Framework{Name: "fastapi", Runtime: "python3.12", Source: "requirements.txt"}, true
		case strings.Contains(content, "django"):
			return Framework{Name: "django", Runtime: "python3.12", Source: "requirements.txt"}, true
		case strings.Contains(content, "flask"):
			return Framework{Name: "flask", Runtime: "python3.12", Source: "requirements.txt"}, true
		}
		return Framework{Name: "python", Runtime: "python3.12", Source: "requirements.txt"}, true
	}
	return Framework{}, false
}

func detectGo(dir string) (Framework, bool) {
	if fileExists(filepath.Join(dir, "go.mod")) {
		return Framework{Name: "go", Runtime: "go1.22", Source: "go.mod"}, true
	}
	return Framework{}, false
}

func detectRust(dir string) (Framework, bool) {
	if fileExists(filepath.Join(dir, "Cargo.toml")) {
		return Framework{Name: "rust", Runtime: "rust1.78", Source: "Cargo.toml"}, true
	}
	return Framework{}, false
}

func detectJava(dir string) (Framework, bool) {
	if matchesGlob(dir, "pom.xml") || matchesGlob(dir, "build.gradle*") {
		if fileExists(filepath.Join(dir, "pom.xml")) {
			data, _ := os.ReadFile(filepath.Join(dir, "pom.xml"))
			if strings.Contains(string(data), "spring-boot") {
				return Framework{Name: "spring-boot", Runtime: "java21", Source: "pom.xml"}, true
			}
		}
		return Framework{Name: "java", Runtime: "java21", Source: "build.gradle"}, true
	}
	return Framework{}, false
}

func detectRuby(dir string) (Framework, bool) {
	if fileExists(filepath.Join(dir, "Gemfile")) {
		return Framework{Name: "ruby", Runtime: "ruby3.3", Source: "Gemfile"}, true
	}
	return Framework{}, false
}

func detectPHP(dir string) (Framework, bool) {
	if fileExists(filepath.Join(dir, "composer.json")) {
		return Framework{Name: "php", Runtime: "php8.3", Source: "composer.json"}, true
	}
	return Framework{}, false
}

func detectStatic(dir string) (Framework, bool) {
	for _, name := range []string{"index.html", "public/index.html", "dist/index.html"} {
		if fileExists(filepath.Join(dir, name)) {
			return Framework{Name: "static", Runtime: "nginx", Source: name}, true
		}
	}
	return Framework{}, false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func matchesGlob(dir, pattern string) bool {
	m, _ := filepath.Glob(filepath.Join(dir, pattern))
	return len(m) > 0
}

func hasDep(deps map[string]string, name string) bool {
	_, ok := deps[name]
	return ok
}

func mergeMaps(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
