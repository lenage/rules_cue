package cuelang

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/iancoleman/strcase"
)

// GenerateRules extracts build metadata from source files in a
// directory.  GenerateRules is called in each directory where an
// update is requested in depth-first post-order.
//
// args contains the arguments for GenerateRules. This is passed as a
// struct to avoid breaking implementations in the future when new
// fields are added.
//
// empty is a list of empty rules that may be deleted after merge.
//
// gen is a list of generated rules that may be updated or added.
//
// Any non-fatal errors this function encounters should be logged
// using log.Print.
func (cl *cueLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	// Get the configuration
	conf := GetConfig(args.Config)

	cueFiles := make(map[string]*ast.File)
	for _, f := range append(args.RegularFiles, args.GenFiles...) {
		// Only generate Cue entries for cue files (.cue)
		if !strings.HasSuffix(f, ".cue") {
			continue
		}

		pth := filepath.Join(args.Dir, f)
		cueFile, err := parser.ParseFile(pth, nil)
		if err != nil {
			log.Printf("parsing cue file: path=%q, err=%+v", pth, err)
			continue
		}
		cueFiles[f] = cueFile
	}

	implicitPkgName := path.Base(args.Rel)
	baseImportPath := computeImportPath(args)

	// categorize cue files into export and library sources
	// cue_libary names are based on cue package name.
	libraries := make(map[string]*cueLibrary)
	exports := make(map[string]*cueExport)

	// For @rules_cue rules
	instances := make(map[string]*cueInstance)
	exportedInstances := make(map[string]*cueExportedInstance)
	exportedFiles := make(map[string]*cueExportedFiles)

	// Check if current directory is cue.mod
	isCueModDir := path.Base(args.Dir) == "cue.mod"

	// Find the nearest cue_module by looking up the directory tree
	moduleLabel := findNearestCueModule(args.Dir, args.Rel)

	for fname, cueFile := range cueFiles {
		pkg := cueFile.PackageName()
		if pkg == "" {
			tgt := exportName(fname)

			// Only create tnarg_rules_cue rules if enabled
			if conf.enableTnargRulesCue {
				export := &cueExport{
					Name:    tgt,
					Src:     fname,
					Imports: make(map[string]bool),
				}
				for _, imprt := range cueFile.Imports {
					imprt := strings.Trim(imprt.Path.Value, "\"")
					export.Imports[imprt] = true
				}
				exports[tgt] = export
			}

			// Always create rules_cue rules
			exportedInstance := &cueExportedInstance{
				Name:    tgt + "_exported",
				Src:     fname,
				Imports: make(map[string]bool),
			}
			for _, imprt := range cueFile.Imports {
				imprt := strings.Trim(imprt.Path.Value, "\"")
				exportedInstance.Imports[imprt] = true
			}
			exportedInstances[exportedInstance.Name] = exportedInstance

			// Also create a cue_exported_files rule
			// exportedFilesName := tgt + "_exported_files"
			// exportedFile := &cueExportedFiles{
			// 	Name:    exportedFilesName,
			// 	Module:  "",
			// 	Imports: make(map[string]bool),
			// }
			// for _, imprt := range cueFile.Imports {
			// 	imprt := strings.Trim(imprt.Path.Value, "\"")
			// 	exportedFile.Imports[imprt] = true
			// }
			// exportedFiles[exportedFilesName] = exportedFile
		} else {
			// For @com_github_tnarg_rules_cue - only if enabled
			if conf.enableTnargRulesCue {
				tgt := fmt.Sprintf("cue_%s_library", pkg)
				lib, ok := libraries[tgt]
				if !ok {
					var importPath string
					if pkg == implicitPkgName {
						importPath = baseImportPath
					} else {
						importPath = fmt.Sprintf("%s:%s", baseImportPath, pkg)
					}
					lib = &cueLibrary{
						Name:       tgt,
						ImportPath: importPath,
						Imports:    make(map[string]bool),
					}
					libraries[tgt] = lib
				}
				lib.Srcs = append(lib.Srcs, fname)
				for _, imprt := range cueFile.Imports {
					imprt := strings.Trim(imprt.Path.Value, "\"")
					lib.Imports[imprt] = true
				}
			}

			// For @rules_cue - always generate these
			instanceTgt := fmt.Sprintf("cue_%s_instance", pkg)
			instance, ok := instances[instanceTgt]
			if !ok {
				instance = &cueInstance{
					Name:        instanceTgt,
					PackageName: pkg,
					Imports:     make(map[string]bool),
					Module:      moduleLabel,
				}
				instances[instanceTgt] = instance
			}
			instance.Srcs = append(instance.Srcs, fname)
			for _, imprt := range cueFile.Imports {
				imprt := strings.Trim(imprt.Path.Value, "\"")
				instance.Imports[imprt] = true
			}

			// Also create a cue_exported_files rule
			// exportedFilesName := fmt.Sprintf("cue_%s_exported_files", pkg)
			// exportedFile, ok := exportedFiles[exportedFilesName]
			// if !ok {
			// 	exportedFile = &cueExportedFiles{
			// 		Name:    exportedFilesName,
			// 		Module:  pkg,
			// 		Imports: make(map[string]bool),
			// 	}
			// 	exportedFiles[exportedFilesName] = exportedFile
			// }
			// for _, imprt := range cueFile.Imports {
			// 	imprt := strings.Trim(imprt.Path.Value, "\"")
			// 	exportedFile.Imports[imprt] = true
			// }
		}
	}

	var res language.GenerateResult

	// Generate cue_module rule if current directory is cue.mod
	if isCueModDir {
		cueModule := &cueModule{
			Name: "cue.mod",
		}
		res.Gen = append(res.Gen, cueModule.ToRule())
	}

	// Generate @com_github_tnarg_rules_cue rules only if enabled
	if conf.enableTnargRulesCue {
		for _, library := range libraries {
			res.Gen = append(res.Gen, library.ToRule())
		}

		for _, export := range exports {
			res.Gen = append(res.Gen, export.ToRule())
		}
	}

	// Generate @rules_cue rules - always generate these
	for _, instance := range instances {
		res.Gen = append(res.Gen, instance.ToRule())

		// Also create a cue_exported_instance rule for each instance
		exportedInstanceName := instance.Name + "_exported"
		exportedInstance := &cueExportedInstance{
			Name:     exportedInstanceName,
			Instance: instance.TargetName(),
			Imports:  instance.Imports,
		}
		res.Gen = append(res.Gen, exportedInstance.ToRule())
	}

	for _, exportedInstance := range exportedInstances {
		res.Gen = append(res.Gen, exportedInstance.ToRule())
	}

	for _, exportedFile := range exportedFiles {
		res.Gen = append(res.Gen, exportedFile.ToRule())
	}

	res.Imports = make([]interface{}, len(res.Gen))
	for i, r := range res.Gen {
		res.Imports[i] = r.PrivateAttr(config.GazelleImportsKey)
	}

	res.Empty = generateEmpty(args.File, libraries, exports, instances, exportedInstances, exportedFiles, conf.enableTnargRulesCue, isCueModDir)

	return res
}

// findNearestCueModule searches for a cue.mod directory up the directory tree
// and returns the label to the cue_module rule.
func findNearestCueModule(dir, rel string) string {
	for currentDir, currentRel := dir, rel; currentRel != ""; {
		cueModPath := filepath.Join(currentDir, "cue.mod")
		if info, err := os.Stat(cueModPath); err == nil && info.IsDir() {
			if currentRel == "." {
				return "//:cue.mod"
			}
			return fmt.Sprintf("//%s/cue.mod:cue.mod", currentRel)
		}
		currentDir = filepath.Dir(currentDir)
		currentRel = filepath.Dir(currentRel)
		if currentRel == "." {
			currentRel = ""
		}
	}
	return ""
}

func computeImportPath(args language.GenerateArgs) string {
	conf := GetConfig(args.Config)

	suffix, err := filepath.Rel(conf.prefixRel, args.Rel)
	if err != nil {
		log.Printf("Failed to compute importpath: rel=%q, prefixRel=%q, err=%+v", args.Rel, conf.prefixRel, err)
		return args.Rel
	}
	if suffix == "." {
		return conf.prefix
	}

	return filepath.Join(conf.prefix, suffix)
}

func exportName(basename string) string {
	parts := strings.Split(basename, ".")
	return strcase.ToSnake(strings.Join(parts[:len(parts)-1], "_"))
}

func generateEmpty(f *rule.File, libraries map[string]*cueLibrary, exports map[string]*cueExport,
	instances map[string]*cueInstance, exportedInstances map[string]*cueExportedInstance,
	exportedFiles map[string]*cueExportedFiles, enableTnargRulesCue bool, isCueModDir bool) []*rule.Rule {
	if f == nil {
		return nil
	}
	var empty []*rule.Rule
	for _, r := range f.Rules {
		switch r.Kind() {
		case "cue_library", "cue_export":
			// Only check these rules if tnarg_rules_cue is enabled
			if enableTnargRulesCue {
				if r.Kind() == "cue_library" {
					if _, ok := libraries[r.Name()]; !ok {
						empty = append(empty, rule.NewRule("cue_library", r.Name()))
					}
				} else { // cue_export
					if _, ok := exports[r.Name()]; !ok {
						empty = append(empty, rule.NewRule("cue_export", r.Name()))
					}
				}
			}
		case "cue_instance":
			if _, ok := instances[r.Name()]; !ok {
				empty = append(empty, rule.NewRule("cue_instance", r.Name()))
			}
		case "cue_exported_instance", "cue_exported_standalone_files":
			if _, ok := exportedInstances[r.Name()]; !ok {
				empty = append(empty, rule.NewRule(r.Kind(), r.Name()))
			}
		case "cue_exported_files":
			if _, ok := exportedFiles[r.Name()]; !ok {
				empty = append(empty, rule.NewRule("cue_exported_files", r.Name()))
			}
		case "cue_module":
			if !isCueModDir {
				empty = append(empty, rule.NewRule("cue_module", r.Name()))
			}
		}
		// Don't mark other rule types as empty
	}
	return empty
}

type cueLibrary struct {
	Name       string
	ImportPath string
	Srcs       []string
	Imports    map[string]bool
}

func (cl *cueLibrary) ToRule() *rule.Rule {
	rule := rule.NewRule("cue_library", cl.Name)
	sort.Strings(cl.Srcs)
	rule.SetAttr("srcs", cl.Srcs)
	rule.SetAttr("visibility", []string{"//visibility:public"})
	rule.SetAttr("importpath", cl.ImportPath)
	var imprts []string
	for imprt := range cl.Imports {
		imprts = append(imprts, imprt)
	}
	sort.Strings(imprts)
	rule.SetPrivateAttr(config.GazelleImportsKey, imprts)
	return rule
}

type cueExport struct {
	Name    string
	Src     string
	Imports map[string]bool
}

func (ce *cueExport) ToRule() *rule.Rule {
	rule := rule.NewRule("cue_export", ce.Name)
	rule.SetAttr("src", ce.Src)
	rule.SetAttr("visibility", []string{"//visibility:public"})
	var imprts []string
	for imprt := range ce.Imports {
		imprts = append(imprts, imprt)
	}
	sort.Strings(imprts)
	rule.SetPrivateAttr(config.GazelleImportsKey, imprts)
	return rule
}

// New types for @rules_cue rules
type cueInstance struct {
	Name        string
	PackageName string
	Srcs        []string
	Imports     map[string]bool
	Module      string // Reference to the nearest cue_module
}

func (ci *cueInstance) ToRule() *rule.Rule {
	rule := rule.NewRule("cue_instance", ci.Name)
	sort.Strings(ci.Srcs)
	rule.SetAttr("srcs", ci.Srcs)
	rule.SetAttr("package_name", ci.PackageName)
	rule.SetAttr("visibility", []string{"//visibility:public"})

	// Set module attribute if a cue_module was found
	if ci.Module != "" {
		rule.SetAttr("ancestor", ci.Module)
	}
	var deps []string
	for dep := range ci.Imports {
		deps = append(deps, dep)
	}
	sort.Strings(deps)
	rule.SetPrivateAttr(config.GazelleImportsKey, deps)
	return rule
}

// Implement TargetName method for cueInstance
func (ci *cueInstance) TargetName() string {
	return ci.Name
}

type cueExportedInstance struct {
	Name     string
	Instance string
	Src      string // Used for standalone files
	Imports  map[string]bool
}

func (cei *cueExportedInstance) ToRule() *rule.Rule {
	var r *rule.Rule
	if cei.Instance != "" {
		r = rule.NewRule("cue_exported_instance", cei.Name)
		r.SetAttr("instance", ":"+cei.Instance)
	} else {
		r = rule.NewRule("cue_exported_standalone_files", cei.Name)
		r.SetAttr("srcs", []string{cei.Src})
	}
	r.SetAttr("visibility", []string{"//visibility:public"})
	var imprts []string
	for imprt := range cei.Imports {
		imprts = append(imprts, imprt)
	}
	sort.Strings(imprts)
	r.SetPrivateAttr(config.GazelleImportsKey, imprts)
	return r
}

type cueExportedFiles struct {
	Name    string
	Module  string
	Imports map[string]bool
}

func (cef *cueExportedFiles) ToRule() *rule.Rule {
	r := rule.NewRule("cue_exported_files", cef.Name)
	r.SetAttr("module", cef.Module)
	r.SetAttr("visibility", []string{"//visibility:public"})
	var imprts []string
	for imprt := range cef.Imports {
		imprts = append(imprts, imprt)
	}
	sort.Strings(imprts)
	r.SetPrivateAttr(config.GazelleImportsKey, imprts)
	return r
}

// Add cue_module type
type cueModule struct {
	Name string
}

func (cm *cueModule) ToRule() *rule.Rule {
	r := rule.NewRule("cue_module", cm.Name)
	r.SetAttr("visibility", []string{"//visibility:public"})
	return r
}
