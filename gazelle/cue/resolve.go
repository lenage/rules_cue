package cuelang

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/pathtools"
	"github.com/bazelbuild/bazel-gazelle/repo"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

// CueModuleInfo stores information about a cue_module
type CueModuleInfo struct {
	Label string // Bazel label of the cue_module
	Dir   string // Directory path where cue.mod is located
}

var (
	cueModulesMu  sync.RWMutex
	cueModules    map[string]*CueModuleInfo            // Maps module label to info
	cueModIndex   map[*CueModuleInfo]map[string]string // Maps CueModuleInfo to import paths to Bazel targets
	debugModIndex bool

	cueModuleKnownDirs = []string{"gen", "usr", "pkg"}
)

// Initialize the maps
func init() {
	cueModules = make(map[string]*CueModuleInfo)
	cueModIndex = make(map[*CueModuleInfo]map[string]string)
	debugModIndex = false
}

// RegisterCueModule registers a cue_module for later use in resolution
func RegisterCueModule(label, dir string) {
	cueModulesMu.Lock()
	defer cueModulesMu.Unlock()

	cueModules[label] = &CueModuleInfo{
		Label: label,
		Dir:   dir,
	}

	// Index all relevant directories if they exist
	for _, subdir := range cueModuleKnownDirs {
		dirPath := filepath.Join(dir, subdir)
		if _, err := os.Stat(dirPath); err == nil {
			indexCueModuleDir(cueModules[label], subdir, dirPath)
		}
	}
}

// indexCueModuleDir builds an index of available targets in a cue.mod subdirectory
func indexCueModuleDir(moduleInfo *CueModuleInfo, subdir, dirPath string) {
	// Initialize the index map for this module if it doesn't exist
	if _, ok := cueModIndex[moduleInfo]; !ok {
		cueModIndex[moduleInfo] = make(map[string]string)
	}

	err := filepath.Walk(dirPath, func(filePath string, fileInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Skip directories
		if fileInfo.IsDir() {
			return nil
		}

		// Only look at .cue files to identify packages
		if !strings.HasSuffix(filePath, ".cue") {
			return nil
		}

		// Get the relative path from the subdirectory
		relPath, err := filepath.Rel(dirPath, filepath.Dir(filePath))
		if err != nil {
			return err
		}

		// Convert to import path format
		importPath := strings.ReplaceAll(relPath, string(filepath.Separator), "/")

		// Extract package name from the file
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}

		// Simple parsing to find package name
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "package ") {
				pkgName := strings.TrimSpace(strings.TrimPrefix(line, "package "))

				// Extract module path from label (removing the target part)
				modulePath := moduleInfo.Label
				if idx := strings.LastIndex(modulePath, ":"); idx >= 0 {
					modulePath = modulePath[:idx]
				}

				// Create target labels for both instance and library with module path prefix
				instanceTarget := fmt.Sprintf("%s/%s/%s:cue_%s_instance", modulePath, subdir, importPath, pkgName)

				// Add to index with full import path
				cueModIndex[moduleInfo][importPath] = instanceTarget

				// Also index by package name for simpler imports
				if pkgName != "" {
					cueModIndex[moduleInfo][pkgName] = instanceTarget
				}

				// Index common domain imports (k8s.io, sigs.k8s.io, github.com)
				for _, prefix := range []string{"k8s.io/", "sigs.k8s.io/", "github.com/"} {
					if strings.Contains(importPath, prefix) {
						parts := strings.SplitN(importPath, "/", 3)
						if len(parts) >= 3 {
							cueModIndex[moduleInfo][strings.Join(parts, "/")] = instanceTarget
						}
						break
					}
				}

				if debugModIndex {
					log.Printf("Debug: Indexed CUE module target: %s -> %s (from %s)", importPath, instanceTarget, subdir)
				}
				break
			}
		}

		return nil
	})

	if err != nil {
		log.Printf("Warning: Error walking cue.mod/%s directory: %v", subdir, err)
	}
}

// Imports returns a list of ImportSpecs that can be used to import
// the rule r. This is used to populate RuleIndex.
//
// If nil is returned, the rule will not be indexed. If any non-nil
// slice is returned, including an empty slice, the rule will be
// indexed.
func (cl *cueLang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
	conf := GetConfig(c)
	switch r.Kind() {
	case "cue_library":
		return []resolve.ImportSpec{
			{
				Lang: cueName,
				Imp:  r.AttrString("importpath"),
			},
		}
	case "cue_instance":
		// For cue_instance, we use the package_name attribute as the import path
		if pkgName := r.AttrString("package_name"); pkgName != "" {
			// Get the configuration to access the prefix
			importPath := pkgName

			// If we have a prefix, use it to form a fully qualified import path
			if conf.prefix != "" && !strings.Contains(pkgName, "/") {
				importPath = path.Join(conf.prefix, pkgName)
			}

			return []resolve.ImportSpec{
				{
					Lang: cueName,
					Imp:  importPath,
				},
			}
		}
	case "cue_module":
		// Register this cue_module for later use in resolution
		if f != nil && f.Pkg != "" {
			moduleLabel := "//" + f.Pkg + ":" + r.Name()
			moduleDir := filepath.Join(c.RepoRoot, f.Pkg)
			RegisterCueModule(moduleLabel, moduleDir)
		}
	}
	return nil
}

// Embeds returns a list of labels of rules that the given rule
// embeds. If a rule is embedded by another importable rule of the
// same language, only the embedding rule will be indexed. The
// embedding rule will inherit the imports of the embedded rule.
func (cl *cueLang) Embeds(r *rule.Rule, from label.Label) []label.Label {
	// Cue doesn't have a concept of embedding as far as I know.
	return nil
}

// Resolve translates imported libraries for a given rule into Bazel
// dependencies. A list of imported libraries is typically stored in a
// private attribute of the rule when it's generated (this interface
// doesn't dictate how that is stored or represented). Resolve
// generates a "deps" attribute (or the appropriate language-specific
// equivalent) for each import according to language-specific rules
// and heuristics.
func (cl *cueLang) Resolve(c *config.Config, ix *resolve.RuleIndex, rc *repo.RemoteCache, r *rule.Rule, imports interface{}, from label.Label) {
	// Get the configuration
	conf := GetConfig(c)

	// Skip resolving imports for tnarg_rules_cue rules if not enabled
	if !conf.enableTnargRulesCue && (r.Kind() == "cue_library" || r.Kind() == "cue_export") {
		return
	}

	if imports == nil {
		return
	}
	imps := imports.([]string)
	r.DelAttr("deps")
	depSet := make(map[string]bool)

	// Get the ancestor attribute for cue_instance or module attribute for cue_exported_files
	var cueModule string
	if r.Kind() == "cue_instance" {
		cueModule = r.AttrString("ancestor")
	} else if r.Kind() == "cue_exported_files" {
		cueModule = r.AttrString("module")
	}

	if debugModIndex {
		log.Printf("DEBUG: kind: %s, cueModule: %s\n", r.Kind(), cueModule)
	}

	for _, imp := range imps {
		if _, ok := stdlib[imp]; ok {
			continue
		}

		// 1. Check cueModelIndex
		if cueModule != "" {
			cueModulesMu.RLock()
			if moduleInfo, ok := cueModules[cueModule]; ok {
				modIndex := cueModIndex[moduleInfo]
				if target, ok := modIndex[imp]; ok {
					if debugModIndex {
						log.Printf("DEBUG: Found import %s in cue.mod index: %s", imp, target)
					}
					depSet[target] = true
					cueModulesMu.RUnlock()
					continue
				}
			}
		} else {
			// 2. If no cueModule, check globally
			cueModulesMu.RLock()
			for _, moduleInfo := range cueModules {
				modIndex := cueModIndex[moduleInfo]
				if target, ok := modIndex[imp]; ok {
					if debugModIndex {
						log.Printf("DEBUG: Found import %s in global cue.mod index: %s", imp, target)
					}
					depSet[target] = true
					continue
				}
			}
			cueModulesMu.RUnlock()
		}

		if debugModIndex {
			log.Printf("%s not found in moudle, try find Rules By Import\n", imp)
		}

		// 3. If not found
		// Try to find rules with matching imports in the index
		// for example @io_k8s_api/apps/v1:cue_v1_instance
		res := ix.FindRulesByImportWithConfig(c,
			resolve.ImportSpec{
				Lang: cueName,
				Imp:  imp,
			}, cueName)
		if len(res) > 0 {
			for _, entry := range res {
				l := entry.Label.Rel(from.Repo, from.Pkg)
				depSet[l.String()] = true
			}
		} else {
			prefix, repo, err := rc.Root(imp)
			if err != nil {
				log.Printf("error resolving %q: %+v", imp, err)
			} else {
				var pkg string
				if pathtools.HasPrefix(imp, prefix) {
					pkg = pathtools.TrimPrefix(imp, prefix)
				}
				if pkg != "" {
					base := path.Base(pkg)
					baseParts := strings.SplitN(base, ":", 2)
					var cuePkg string
					if len(baseParts) > 1 {
						cuePkg = baseParts[1]
					} else {
						cuePkg = base
					}

					instanceLabel := label.New(repo, path.Join(path.Dir(pkg), baseParts[0]), fmt.Sprintf("cue_%s_instance", cuePkg))

					// Prefer cue_instance if we're using the new rules
					// Check if the rule kind is one that should use instance labels
					instanceKinds := map[string]bool{
						"cue_exported_files":            true,
						"cue_instance":                  true,
						"cue_exported_instance":         true,
						"cue_exported_standalone_files": true,
						"cue_consolidated_instance":     true,
					}

					if instanceKinds[r.Kind()] {
						depSet[instanceLabel.String()] = true
					}
				}
			}
		}
	}

	if len(depSet) > 0 {
		deps := make([]string, 0, len(depSet))
		for dep := range depSet {
			deps = append(deps, dep)
		}
		sort.Strings(deps)
		r.SetAttr("deps", deps)
	}
}

var stdlib = map[string]bool{
	"crypto/md5":      true,
	"crypto/sha1":     true,
	"crypto/sha256":   true,
	"crypto/sha512":   true,
	"encoding/base64": true,
	"encoding/csv":    true,
	"encoding/hex":    true,
	"encoding/json":   true,
	"encoding/yaml":   true,
	"html":            true,
	"list":            true,
	"math":            true,
	"math/bits":       true,
	"net":             true,
	"path":            true,
	"regexp":          true,
	"strconv":         true,
	"strings":         true,
	"struct":          true,
	"text/tabwriter":  true,
	"text/template":   true,
	"time":            true,
	"tool":            true,
	"tool/cli":        true,
	"tool/exec":       true,
	"tool/file":       true,
	"tool/http":       true,
	"tool/os":         true,
	// New in CUE 0.12
	"crypto/hmac":     true,
	"encoding/binary": true,
	"encoding/pem":    true,
	"io":              true,
	"math/rand":       true,
	"net/url":         true,
	"path/filepath":   true,
	"uuid":            true,
}
