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

	"cuelang.org/go/cue/parser"
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
	debugModIndex = true
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

		// Parse the CUE file using the CUE parser
		cueFile, err := parser.ParseFile(filePath, nil)
		if err != nil {
			log.Printf("Warning: Error parsing CUE file %s: %v", filePath, err)
			return nil
		}

		// Extract package name directly from the AST
		pkgName := cueFile.PackageName()
		if pkgName == "" {
			// Skip files without package declarations
			return nil
		}

		// Extract module path from label (removing the target part)
		modulePath := moduleInfo.Label
		if idx := strings.LastIndex(modulePath, ":"); idx >= 0 {
			modulePath = modulePath[:idx]
		}

		// Create target label with module path prefix
		instanceTarget := fmt.Sprintf("%s/%s/%s:%s_cue_instance", modulePath, subdir, importPath, pkgName)

		// Index by different references
		indexReferences(moduleInfo, importPath, pkgName, instanceTarget)

		return nil
	})

	if err != nil {
		log.Printf("Warning: Error walking cue.mod/%s directory: %v", subdir, err)
	}
}

// indexReferences adds various import path patterns to the index
func indexReferences(moduleInfo *CueModuleInfo, importPath, pkgName, instanceTarget string) {
	// 1. Index by full import path
	cueModIndex[moduleInfo][importPath] = instanceTarget

	// 2. Index by just package name for simple imports
	if pkgName != "" {
		cueModIndex[moduleInfo][pkgName] = instanceTarget
	}

	// 3. Index all possible colon-style imports (path prefixes + pkg)
	indexColonStyles(moduleInfo, importPath, pkgName, instanceTarget)

	// 4. Index domain-specific imports
	indexDomainSpecific(moduleInfo, importPath, pkgName, instanceTarget)

	if debugModIndex {
		log.Printf("Debug: Indexed CUE module target: %s -> %s", importPath, instanceTarget)
	}
}

// indexColonStyles adds colon-style import references to the index
func indexColonStyles(moduleInfo *CueModuleInfo, importPath, pkgName, instanceTarget string) {
	// Add colon-style import paths for each directory level
	// This allows imports like "domain.com/pkg/path:pkg" to be resolved
	pathParts := strings.Split(importPath, "/")
	for i := 0; i < len(pathParts); i++ {
		if i > 0 {
			// For each valid path prefix, create a colon import with the package name
			prefix := strings.Join(pathParts[:i+1], "/")
			colonImport := fmt.Sprintf("%s:%s", prefix, pkgName)
			cueModIndex[moduleInfo][colonImport] = instanceTarget

			if debugModIndex {
				log.Printf("Debug: Added colon import index: %s -> %s", colonImport, instanceTarget)
			}
		}
	}

	// Add full path with colon to support imports like "full/import/path:pkg"
	fullColonImport := fmt.Sprintf("%s:%s", importPath, pkgName)
	cueModIndex[moduleInfo][fullColonImport] = instanceTarget

	if debugModIndex {
		log.Printf("Debug: Added full colon import index: %s -> %s", fullColonImport, instanceTarget)
	}
}

// indexDomainSpecific adds domain-specific import references to the index
func indexDomainSpecific(moduleInfo *CueModuleInfo, importPath, pkgName, instanceTarget string) {
	// Index common domain imports (k8s.io, sigs.k8s.io, github.com, etc.)
	for _, prefix := range []string{"k8s.io/", "sigs.k8s.io/", "github.com/", "nvda.ai/"} {
		if strings.HasPrefix(importPath, prefix) {
			parts := strings.SplitN(importPath, "/", 3)
			if len(parts) >= 3 {
				// Index the domain import
				domainImport := strings.Join(parts, "/")
				cueModIndex[moduleInfo][domainImport] = instanceTarget

				// Add colon-style import for domain
				colonDomainImport := fmt.Sprintf("%s:%s", domainImport, pkgName)
				cueModIndex[moduleInfo][colonDomainImport] = instanceTarget

				if debugModIndex {
					log.Printf("Debug: Added domain import index: %s -> %s", domainImport, instanceTarget)
					log.Printf("Debug: Added domain colon import index: %s -> %s", colonDomainImport, instanceTarget)
				}
				break
			}
		}
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
		// Only index cue_library rules if tnarg_rules_cue is enabled
		if !conf.enableTnargRulesCue {
			return nil
		}

		importPath := r.AttrString("importpath")
		if importPath == "" {
			return nil
		}

		return []resolve.ImportSpec{
			{
				Lang: cueName,
				Imp:  importPath,
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

			// Create the primary import spec
			specs := []resolve.ImportSpec{
				{
					Lang: cueName,
					Imp:  importPath,
				},
			}

			// Additionally create a colon-style import spec
			// This enables other rules to import this instance with a colon
			parts := strings.Split(importPath, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]

				// If the last part isn't the package name itself, create a colon import
				if lastPart != pkgName {
					basePath := strings.TrimSuffix(importPath, "/"+lastPart)
					colonImport := fmt.Sprintf("%s:%s", basePath, pkgName)

					specs = append(specs, resolve.ImportSpec{
						Lang: cueName,
						Imp:  colonImport,
					})
				}

				// Also handle the full path with colon
				colonFullImport := fmt.Sprintf("%s:%s", importPath, pkgName)
				if colonFullImport != importPath {
					specs = append(specs, resolve.ImportSpec{
						Lang: cueName,
						Imp:  colonFullImport,
					})
				}
			}

			return specs
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

	for _, imp := range imps {
		// Skip standard library imports
		if _, ok := stdlib[imp]; ok {
			continue
		}

		if debugModIndex {
			log.Printf("DEBUG Resolve: kind: %s, cueModule: %s, import_path: %s\n", r.Kind(), cueModule, imp)
		}

		// Stage 1: Try to find the import in cue.mod indexes
		if resolved := tryResolveFromModuleIndex(imp, cueModule, depSet, debugModIndex); resolved {
			continue
		}

		// Stage 2: Try to find in rule index
		if resolved := tryResolveFromRuleIndex(c, ix, imp, from, depSet, debugModIndex); resolved {
			continue
		}

		// Stage 3: Resort to creating labels based on import path
		createLabelFromImportPath(imp, rc, depSet, r.Kind(), debugModIndex)
	}

	if len(depSet) > 0 {
		deps := make([]string, 0, len(depSet))
		for dep := range depSet {
			if debugModIndex {
				log.Printf("Resolve: Adding dependency: %s", dep)
			}
			deps = append(deps, dep)
		}
		sort.Strings(deps)
		r.SetAttr("deps", deps)
	}
}

// tryResolveFromModuleIndex checks if the import exists in cue module indexes
func tryResolveFromModuleIndex(imp, cueModule string, depSet map[string]bool, debugMode bool) bool {
	// Case 1: Check specific module if provided
	if cueModule != "" {
		cueModulesMu.RLock()
		defer cueModulesMu.RUnlock()

		if moduleInfo, ok := cueModules[cueModule]; ok {
			modIndex := cueModIndex[moduleInfo]
			if target, ok := modIndex[imp]; ok {
				if debugMode {
					log.Printf("DEBUG: Found import %s in cue.mod index: %s", imp, target)
				}
				depSet[target] = true
				return true
			}
		}
		return false
	}

	// Case 2: Check all modules if no specific module provided
	cueModulesMu.RLock()
	defer cueModulesMu.RUnlock()

	for _, moduleInfo := range cueModules {
		modIndex := cueModIndex[moduleInfo]
		if target, ok := modIndex[imp]; ok {
			if debugMode {
				log.Printf("DEBUG: Found import %s in global cue.mod index: %s", imp, target)
			}
			depSet[target] = true
			return true
		}
	}

	if debugMode {
		log.Printf("%s not found in module, trying rule index next\n", imp)
	}
	return false
}

// tryResolveFromRuleIndex tries to find the import in the rule index
func tryResolveFromRuleIndex(c *config.Config, ix *resolve.RuleIndex, imp string, from label.Label, depSet map[string]bool, debugMode bool) bool {
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
		return true
	}
	return false
}

// createLabelFromImportPath creates a label based on the import path
func createLabelFromImportPath(imp string, rc *repo.RemoteCache, depSet map[string]bool, ruleKind string, debugMode bool) {
	// Check if the import has a colon (like "path/to/pkg:pkgname")
	colonIndex := strings.LastIndex(imp, ":")
	hasColon := colonIndex >= 0

	// Extract path and package name
	var pathPart, pkgName string
	var importToResolve string

	if hasColon {
		pathPart = imp[:colonIndex]
		pkgName = imp[colonIndex+1:]
		importToResolve = pathPart // Use the path part for root resolution
		if debugMode {
			log.Printf("DEBUG: Processing colon import: path=%s, pkg=%s", pathPart, pkgName)
		}
	} else {
		importToResolve = imp
	}

	prefix, repo, err := rc.Root(importToResolve)
	// Handle error from rc.Root()
	if err != nil {
		log.Printf("Error resolving import %q: %v", importToResolve, err)
		return
	}

	// rc.Root succeeded - create appropriate label
	if hasColon {
		// For colon imports, trim prefix from path part if needed
		if pathtools.HasPrefix(pathPart, prefix) {
			pathPart = pathtools.TrimPrefix(pathPart, prefix)
		}
		instanceLabel := label.New(repo, pathPart, fmt.Sprintf("%s_cue_instance", pkgName))
		if isInstanceKind(ruleKind) {
			depSet[instanceLabel.String()] = true
			if debugMode {
				log.Printf("DEBUG: Created dependency for colon import %s: %s", imp, instanceLabel.String())
			}
		}
	} else {
		// Regular import (no colon)
		var pkg string
		if pathtools.HasPrefix(imp, prefix) {
			pkg = pathtools.TrimPrefix(imp, prefix)
		}

		if pkg != "" {
			cuePkg := path.Base(pkg)
			instanceLabel := label.New(repo, pkg, fmt.Sprintf("%s_cue_instance", cuePkg))

			if isInstanceKind(ruleKind) {
				depSet[instanceLabel.String()] = true
				if debugMode {
					log.Printf("DEBUG: Created dependency for regular import %s: %s", imp, instanceLabel.String())
				}
			}
		}
	}
}

// Helper function to check if a rule kind should use instance labels
func isInstanceKind(kind string) bool {
	instanceKinds := map[string]bool{
		"cue_exported_files":            true,
		"cue_instance":                  true,
		"cue_exported_instance":         true,
		"cue_exported_standalone_files": true,
		"cue_consolidated_instance":     true,
	}
	return instanceKinds[kind]
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
