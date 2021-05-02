package cuelang

import (
	"fmt"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

const (
	cueName = "cue"
)

var _ = fmt.Printf

type cueLang struct{}

// NewLanguage returns an instace of the Gazelle plugin for rules_cue.
func NewLanguage() language.Language {
	return &cueLang{}
}

func (cl *cueLang) Name() string { return cueName }

// Kinds returns a map of maps rule names (kinds) and information on
// how to match and merge attributes that may be found in rules of
// those kinds. All kinds of rules generated for this language may be
// found here.
func (cl *cueLang) Kinds() map[string]rule.KindInfo {
	return map[string]rule.KindInfo{
		"cue_library": {
			MatchAttrs: []string{"import_path"},
			NonEmptyAttrs: map[string]bool{
				"deps": true,
				"srcs": true,
			},
			MergeableAttrs: map[string]bool{
				"srcs":        true,
				"import_path": true,
			},
			ResolveAttrs: map[string]bool{"deps": true},
		},
		"cue_export": {
			NonEmptyAttrs: map[string]bool{
				"deps": true,
				"src":  true,
			},
			MergeableAttrs: map[string]bool{
				"escape":        true,
				"output_format": true,
				"src":           true,
			},
			ResolveAttrs: map[string]bool{"deps": true},
		},
		"cue_repository": {
			MatchAttrs: []string{"importpath"},
			NonEmptyAttrs: map[string]bool{
				"import_path": true,
				"sha256":      true,
				"urls":        true,
			},
			MergeableAttrs: map[string]bool{
				"import_path":  true,
				"sha256":       true,
				"strip_prefix": true,
				"type":         true,
				"urls":         true,
			},
		},
	}
}

// Loads returns .bzl files and symbols they define. Every rule
// generated by GenerateRules, now or in the past, should be loadable
// from one of these files.
func (cl *cueLang) Loads() []rule.LoadInfo {
	return []rule.LoadInfo{
		{
			Name: "@com_github_tnarg_rules_cue//cue:deps.bzl",
			Symbols: []string{
				"cue_register_toolchains",
			},
			After: []string{
				"gazelle_dependencies",
			},
		}, {
			Name: "@com_github_tnarg_rules_cue//cue:cue.bzl",
			Symbols: []string{
				"cue_export",
				"cue_library",
				"cue_repository",
			},
			After: []string{
				"cue_register_toolchains",
			},
		},
	}
}

// Fix repairs deprecated usage of language-specific rules in f. This
// is called before the file is indexed. Unless c.ShouldFix is true,
// fixes that delete or rename rules should not be performed.
func (cl *cueLang) Fix(c *config.Config, f *rule.File) {
	// Currently a noop
}
