// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"net/http"
	"strings"

	"github.com/fodydev/fodycloud/internal/service"
	"github.com/fodydev/fodycloud/internal/stacks"
)

// stackForm is the state of the language/framework picker.
type stackForm struct {
	App       string
	Languages []stacks.Language
	Language  stacks.Language
	Preset    stacks.Preset
	Stack     stacks.Stack // values shown in the fields
}

// newStackForm picks what the form shows: the requested language/framework
// (htmx swaps), else the confirmed stack, else the detected suggestion. The
// commands are those of the starting stack when it is still selected, or the
// preset's defaults after the student switched framework.
func newStackForm(app *service.App, language, framework string, rootDir *string) stackForm {
	f := stackForm{App: app.Name, Languages: stacks.Catalog}
	base := app.Stack
	if !app.StackConfirmed {
		base = stacks.Stack{RootDir: app.Stack.RootDir}
		if app.Detection != nil && app.Detection.Suggested != nil {
			base = *app.Detection.Suggested
		}
	}
	if language == "" {
		language, framework = base.Language, base.Framework
	}
	lang, preset, ok := stacks.Find(language, framework)
	if lang.ID == "" { // nothing chosen yet: the form asks for a language first
		f.Stack.RootDir = base.RootDir
		if rootDir != nil {
			f.Stack.RootDir = strings.TrimSpace(*rootDir)
		}
		return f
	}
	if !ok {
		preset = lang.Presets[0]
	}
	f.Language, f.Preset = lang, preset
	if base.Language == lang.ID && base.Framework == preset.ID {
		f.Stack = base
	} else {
		f.Stack = stacks.Stack{Language: lang.ID, Framework: preset.ID, BuildCommand: preset.Build, StartCommand: preset.Start, PublishDir: preset.PublishDir,
			RootDir: base.RootDir}
	}
	if rootDir != nil { // keep what the student typed across htmx swaps
		f.Stack.RootDir = strings.TrimSpace(*rootDir)
	}
	return f
}

// formPtr returns a form field, or nil when the form doesn't have it.
func formPtr(r *http.Request, key string) *string {
	r.ParseForm() //nolint:errcheck
	if _, ok := r.Form[key]; !ok {
		return nil
	}
	v := r.Form.Get(key)
	return &v
}
