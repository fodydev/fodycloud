// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package stacks is the catalog of languages and frameworks a student can
// choose for an app, the mapping from Railpack's detection to that catalog,
// and the translation of a chosen stack into Railpack options.
package stacks

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"
)

// Stack is what an app is built with: chosen by the student, usually from
// the detected suggestion, and validated by Normalize.
type Stack struct {
	Language     string `json:"language"`
	Framework    string `json:"framework"`
	Version      string `json:"version,omitempty"`       // language version; empty = Railpack's choice
	BuildCommand string `json:"build_command,omitempty"` // empty = the framework's default build
	StartCommand string `json:"start_command,omitempty"` // empty = the framework's default start
	PublishDir   string `json:"publish_dir,omitempty"`   // static output, for static presets (relative to RootDir)
	// RootDir is the working directory inside the repository (e.g. a
	// monorepo's frontend/). Empty means the repository root.
	RootDir string `json:"root_dir,omitempty"`
}

// Preset is a framework of a language, with editable default commands.
type Preset struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
	// Build and Start prefill the form (and are used when a stack leaves them empty).
	Build string `json:"build_command,omitempty"`
	Start string `json:"start_command,omitempty"`
	// Static presets serve PublishDir as files (no run command).
	Static     bool   `json:"static,omitempty"`
	PublishDir string `json:"publish_dir,omitempty"`
	// DocRoot presets are served by the language's own web server (PHP's
	// FrankenPHP): no run command, and PublishDir is the document root.
	DocRoot bool `json:"doc_root,omitempty"`
	// Custom presets require the student's own run command.
	Custom bool `json:"custom,omitempty"`
	// Packages are extra Railpack packages to install (e.g. Bun).
	Packages map[string]string `json:"-"`
}

// Language is a runtime with its Railpack provider and presets.
type Language struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Provider string   `json:"-"`
	Package  string   `json:"-"` // Railpack package name for version pinning
	Versions []string `json:"versions,omitempty"`
	Presets  []Preset `json:"frameworks"`
	// TrustStart copies Railpack's detected start command into the
	// suggestion. Off where it usually comes from a Procfile written for
	// another platform (Go: a binary name; PHP: heroku-php-apache2).
	TrustStart bool `json:"-"`
	// ServedBy is the start command of languages served by a built-in web
	// server (PHP); the student gives no run command.
	ServedBy string `json:"-"`
	// DeployVars are default environment variables of the app's image; the
	// app's own variables override them.
	DeployVars map[string]string `json:"-"`
}

// Catalog lists the stacks offered in the dashboard, API and MCP.
var Catalog = []Language{
	{
		ID: "static", Label: "Static website", Provider: "staticfile",
		Presets: []Preset{
			{ID: "static", Label: "Static files (HTML, CSS, JS)", Static: true, PublishDir: ".",
				Hint: "Files are served as-is by a web server; no build or run command. Set the folder that contains index.html."},
		},
	},
	{
		ID: "python", Label: "Python", Provider: "python", Package: "python", TrustStart: true,
		Versions: []string{"3.13", "3.12", "3.11", "3.10"},
		Presets: []Preset{
			{ID: "flask", Label: "Flask", Build: "pip install gunicorn", Start: "gunicorn --bind 0.0.0.0:$PORT app:app",
				Hint: "Replace app:app with <module>:<Flask object>, e.g. run:app for run.py."},
			{ID: "fastapi", Label: "FastAPI", Build: "pip install uvicorn", Start: "uvicorn main:app --host 0.0.0.0 --port $PORT",
				Hint: "Replace main:app with <module>:<FastAPI object>."},
			{ID: "django", Label: "Django", Build: "pip install gunicorn",
				Start: "python manage.py migrate && gunicorn mysite.wsgi --bind 0.0.0.0:$PORT",
				Hint:  "Replace mysite with your Django project package (the folder containing wsgi.py)."},
			{ID: "fasthtml", Label: "FastHTML", Build: "pip install uvicorn", Start: "uvicorn main:app --host 0.0.0.0 --port $PORT"},
			{ID: "custom", Label: "Other (custom commands)", Custom: true,
				Hint: "Dependencies from requirements.txt / pyproject are installed automatically; give the command that starts your server on $PORT."},
		},
	},
	{
		ID: "node", Label: "Node.js", Provider: "node", Package: "node", TrustStart: true,
		Versions: []string{"24", "22", "20", "18"},
		Presets: []Preset{
			{ID: "node-server", Label: "Node server (Express, Fastify…)", Start: "npm start",
				Hint: "Runs the start script of package.json; the server must listen on $PORT."},
			{ID: "nextjs", Label: "Next.js", Build: "npm run build", Start: "npm start"},
			{ID: "nuxt", Label: "Nuxt", Build: "npm run build", Start: "node .output/server/index.mjs"},
			{ID: "vite", Label: "Vite / React single-page app", Build: "npm run build", Static: true, PublishDir: "dist",
				Hint: "Built with npm run build, then the output folder is served as static files."},
			{ID: "astro", Label: "Astro (static)", Build: "npm run build", Static: true, PublishDir: "dist"},
			{ID: "bun", Label: "Bun server", Start: "bun run start", Packages: map[string]string{"bun": "latest"},
				Hint: "Runs the start script of package.json with Bun; the server must listen on $PORT (Bun.serve uses it by default)."},
			{ID: "custom", Label: "Other (custom commands)", Custom: true,
				Hint: "Dependencies are installed automatically; give the command that starts your server on $PORT."},
		},
	},
	{
		// No version choice: Railpack picks the FrankenPHP image from
		// composer.json's require.php. Pinning a version would make it compile
		// PHP from source instead.
		ID: "php", Label: "PHP", Provider: "php", ServedBy: "/start-container.sh",
		Presets: []Preset{
			{ID: "php", Label: "PHP (plain PHP, Slim, Symfony…)", DocRoot: true, PublishDir: ".",
				Hint: "Served by FrankenPHP; Composer dependencies are installed automatically. Set the publish folder to the one that contains index.php, e.g. public or web."},
			{ID: "laravel", Label: "Laravel",
				Hint: "Set APP_KEY in the environment variables (php artisan key:generate --show). Migrations run at each start; a SQLite database is reset on each deployment."},
		},
	},
	{
		ID: "go", Label: "Go", Provider: "golang", Package: "go",
		Versions: []string{"1.27", "1.26", "1.25"},
		Presets: []Preset{
			{ID: "go-server", Label: "Go server (net/http, Gin, Echo…)", Start: "./out",
				Hint: "Built with go build into ./out. The server must listen on $PORT."},
			{ID: "custom", Label: "Other (custom commands)", Custom: true,
				Hint: "For a main package in a subfolder: build with go build -o out ./cmd/server, run ./out. The server must listen on $PORT."},
		},
	},
	{
		// Railpack's Gradle start command looks for */build/libs (subprojects),
		// which misses single-project builds: FodyCloud runs the jar itself.
		ID: "java", Label: "Java", Provider: "java", Package: "java",
		Versions: []string{"25", "21", "17"},
		// The JVM sizes its heap to 25% of the container's memory by default.
		DeployVars: map[string]string{"JAVA_OPTS": "-XX:MaxRAMPercentage=75.0"},
		Presets: []Preset{
			{ID: "spring-boot", Label: "Spring Boot (Maven or Gradle)", Start: "java $JAVA_OPTS -Dserver.port=$PORT -jar " + javaJar,
				Hint: "Built with Maven or Gradle; runs the jar from target/ or build/libs/. Spring Boot needs about 300 MB of memory: the free plan's 256 MB may be too small."},
			{ID: "jar", Label: "Java application (JAR)", Start: "java $JAVA_OPTS -jar " + javaJar,
				Hint: "Built with Maven or Gradle; runs the jar from target/ or build/libs/. The app must listen on the port in the PORT environment variable."},
			{ID: "custom", Label: "Other (custom commands)", Custom: true,
				Hint: "Give the command that starts your server on $PORT, e.g. java $JAVA_OPTS -jar target/app.jar."},
		},
	},
}

// javaJar finds the jar of a single-project Maven (target/) or Gradle
// (build/libs/) build, skipping Gradle's "-plain" jar without dependencies.
const javaJar = "$(ls -1 target/*.jar build/libs/*.jar 2>/dev/null | grep -v -- -plain.jar | head -1)"

// Find returns a language and one of its presets.
func Find(language, framework string) (Language, Preset, bool) {
	for _, l := range Catalog {
		if l.ID != language {
			continue
		}
		for _, p := range l.Presets {
			if p.ID == framework {
				return l, p, true
			}
		}
		return l, Preset{}, false
	}
	return Language{}, Preset{}, false
}

// InvalidError is a user-facing validation error.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return &InvalidError{Msg: fmt.Sprintf(format, a...)} }

// MaxCommandLen bounds build and run commands.
const MaxCommandLen = 2000

var publishDir = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

func safeRelPath(p string) bool {
	return publishDir.MatchString(p) && !slices.Contains(strings.Split(p, "/"), "..") && !strings.HasPrefix(p, "-")
}

// CleanRootDir validates a working directory: a relative folder inside the
// repository. "", "." and "/" mean the repository root and return "".
func CleanRootDir(dir string) (string, error) {
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	for strings.HasPrefix(dir, "./") {
		dir = strings.TrimLeft(strings.TrimPrefix(dir, "./"), "/")
	}
	if dir == "" || dir == "." {
		return "", nil
	}
	if !safeRelPath(dir) {
		return "", invalid("working directory must be a folder inside the repository, e.g. frontend or apps/web")
	}
	return dir, nil
}

// Normalize validates a chosen stack against the catalog and fills the
// preset defaults for anything left empty.
func (s *Stack) Normalize() error {
	s.Language = strings.ToLower(strings.TrimSpace(s.Language))
	s.Framework = strings.ToLower(strings.TrimSpace(s.Framework))
	s.Version = strings.TrimSpace(s.Version)
	s.BuildCommand = strings.TrimSpace(s.BuildCommand)
	s.StartCommand = strings.TrimSpace(s.StartCommand)
	s.PublishDir = strings.Trim(strings.TrimSpace(s.PublishDir), "/")
	root, err := CleanRootDir(s.RootDir)
	if err != nil {
		return err
	}
	s.RootDir = root
	lang, preset, ok := Find(s.Language, s.Framework)
	if lang.ID == "" {
		return invalid("unknown language %q (available: %s)", s.Language, strings.Join(languageIDs(), ", "))
	}
	if !ok {
		return invalid("unknown framework %q for %s (available: %s)", s.Framework, lang.Label, strings.Join(presetIDs(lang), ", "))
	}
	if s.Version != "" && !slices.Contains(lang.Versions, s.Version) {
		if len(lang.Versions) == 0 {
			return invalid("%s has no selectable version", lang.Label)
		}
		return invalid("unsupported %s version %q (available: %s)", lang.Label, s.Version, strings.Join(lang.Versions, ", "))
	}
	for name, c := range map[string]string{"build command": s.BuildCommand, "run command": s.StartCommand} {
		if len(c) > MaxCommandLen {
			return invalid("%s is too long (max %d characters)", name, MaxCommandLen)
		}
		if strings.ContainsAny(c, "\x00\r\n") {
			return invalid("%s must be a single line (chain steps with &&)", name)
		}
	}
	if preset.Static && s.StartCommand != "" {
		return invalid("%s is served as static files: it has no run command", preset.Label)
	}
	if lang.ServedBy != "" && s.StartCommand != "" {
		return invalid("%s apps are served by FrankenPHP: they have no run command", lang.Label)
	}
	if preset.Static || preset.DocRoot {
		if s.PublishDir == "" {
			s.PublishDir = preset.PublishDir
		}
		if s.PublishDir == "" {
			s.PublishDir = "."
		}
		if !safeRelPath(s.PublishDir) {
			return invalid("publish folder must be a relative path inside the repository, e.g. dist or public")
		}
	} else {
		s.PublishDir = ""
	}
	if preset.Custom && s.StartCommand == "" {
		return invalid("custom commands need a run command that starts a server listening on $PORT")
	}
	if s.BuildCommand == "" {
		s.BuildCommand = preset.Build
	}
	if s.StartCommand == "" {
		s.StartCommand = preset.Start
	}
	return nil
}

func languageIDs() []string {
	ids := make([]string, len(Catalog))
	for i, l := range Catalog {
		ids[i] = l.ID
	}
	return ids
}

func presetIDs(l Language) []string {
	ids := make([]string, len(l.Presets))
	for i, p := range l.Presets {
		ids[i] = p.ID
	}
	return ids
}

// Options is how a stack is passed to `railpack prepare`.
type Options struct {
	Config       []byte            // content of a Railpack config file (nil = none)
	BuildCommand string            // --build-cmd
	StartCommand string            // --start-cmd
	Env          map[string]string // Railpack configuration variables (--env)
	RequireStart bool              // --error-missing-start
}

// Railpack translates a normalized stack into Railpack options.
func (s Stack) Railpack() Options {
	o := Options{BuildCommand: s.BuildCommand, StartCommand: s.StartCommand, Env: map[string]string{}, RequireStart: true}
	lang, preset, _ := Find(s.Language, s.Framework)
	cfg := map[string]any{"provider": lang.Provider}
	pkgs := map[string]string{}
	if s.Version != "" && lang.Package != "" {
		pkgs[lang.Package] = s.Version
	}
	for k, v := range preset.Packages {
		pkgs[k] = v
	}
	if len(pkgs) > 0 {
		cfg["packages"] = pkgs
	}
	if len(lang.DeployVars) > 0 {
		cfg["deploy"] = map[string]any{"variables": lang.DeployVars}
	}
	o.Config, _ = json.Marshal(cfg)
	if lang.ServedBy != "" {
		// Always the built-in server: a Procfile's command (for example
		// heroku-php-apache2) doesn't exist in the image.
		o.StartCommand = lang.ServedBy
		if preset.DocRoot {
			root := "/app" // the working directory in the image
			if s.PublishDir != "." && s.PublishDir != "" {
				root += "/" + s.PublishDir
			}
			o.Env["RAILPACK_PHP_ROOT_DIR"] = root // must be absolute
		}
	}
	if preset.Static {
		o.RequireStart = false
		o.StartCommand = "" // a start command would disable static serving
		if lang.Provider == "staticfile" {
			o.Env["RAILPACK_STATIC_FILE_ROOT"] = s.PublishDir
		} else {
			o.Env["RAILPACK_SPA_OUTPUT_DIR"] = s.PublishDir
		}
	}
	return o
}

// Detection is what FodyCloud found in a repository before the first build.
type Detection struct {
	Detected  bool     `json:"detected"`
	Language  string   `json:"language,omitempty"`  // catalog id, if supported
	Framework string   `json:"framework,omitempty"` // catalog id, if recognised
	Version   string   `json:"version,omitempty"`   // resolved language version
	Provider  string   `json:"provider,omitempty"`  // raw Railpack provider
	Runtime   string   `json:"runtime,omitempty"`   // raw Railpack runtime/framework hint
	RootDir   string   `json:"root_dir,omitempty"`  // subfolder where the stack was found ("" = repository root)
	Suggested *Stack   `json:"suggested,omitempty"` // prefilled choice for the review form
	Message   string   `json:"message"`
	Logs      []string `json:"logs,omitempty"`
}

type railpackInfo struct {
	Success           bool              `json:"success"`
	DetectedProviders []string          `json:"detectedProviders"`
	Metadata          map[string]string `json:"metadata"`
	ResolvedPackages  map[string]struct {
		ResolvedVersion string `json:"resolvedVersion"`
	} `json:"resolvedPackages"`
	Logs []struct {
		Level string `json:"level"`
		Msg   string `json:"msg"`
	} `json:"logs"`
}

type railpackPlan struct {
	Deploy struct {
		StartCommand string `json:"startCommand"`
	} `json:"deploy"`
}

var staticRootLog = regexp.MustCompile(`(?i)staticfile root dir:\s*(\S+)`)

// Label is the human name of a stack, e.g. "Python · Flask".
func Label(s Stack) string { return labelOf(s.Language, s.Framework) }

// Detect maps Railpack's info and plan files to the catalog. files is the
// analysed folder (nil = none), read for hints Railpack doesn't report: a PHP
// app's document root, the Java version of a Maven or Gradle build.
func Detect(info, plan []byte, files fs.FS) Detection {
	if files == nil {
		files = noFiles{}
	}
	var in railpackInfo
	_ = json.Unmarshal(info, &in)
	var pl railpackPlan
	_ = json.Unmarshal(plan, &pl)
	d := Detection{}
	staticRoot := ""
	for _, l := range in.Logs {
		if l.Msg == "" {
			continue
		}
		d.Logs = append(d.Logs, l.Msg)
		if m := staticRootLog.FindStringSubmatch(l.Msg); m != nil {
			staticRoot = m[1]
		}
	}
	if len(in.DetectedProviders) == 0 {
		d.Message = "We couldn't detect how to build this repository. Choose its language and framework below."
		return d
	}
	d.Provider = in.DetectedProviders[0]
	for _, l := range Catalog {
		if l.Provider == d.Provider {
			d.Language = l.ID
			if l.Package != "" {
				d.Version = majorMinor(in.ResolvedPackages[l.Package].ResolvedVersion, l.Versions)
			}
		}
	}
	if d.Language == "" {
		d.Message = fmt.Sprintf("Detected %s, which FodyCloud doesn't offer yet. Choose a supported stack below.", d.Provider)
		return d
	}
	d.Detected = true
	switch d.Language {
	case "static":
		d.Framework = "static"
	case "python":
		d.Runtime = in.Metadata["pythonRuntime"]
		if _, _, ok := Find("python", d.Runtime); ok && d.Runtime != "" {
			d.Framework = d.Runtime
		}
	case "node":
		d.Runtime = in.Metadata["nodeRuntime"]
		switch {
		case d.Runtime == "next":
			d.Framework = "nextjs"
		case d.Runtime == "nuxt":
			d.Framework = "nuxt"
		case d.Runtime == "astro":
			d.Framework = "astro"
		case d.Runtime == "vite" || in.Metadata["nodeSPAFramework"] != "":
			d.Framework = "vite"
		case d.Runtime == "bun":
			d.Framework = "bun"
		default:
			d.Framework = "node-server"
		}
	case "php":
		d.Runtime = in.Metadata["phpFramework"]
		d.Framework = "php"
		if d.Runtime == "laravel" {
			d.Framework = "laravel"
		}
	case "go":
		d.Framework = "go-server"
	case "java":
		d.Runtime = in.Metadata["javaPackageManager"]
		if v := javaVersion(files); slices.Contains(lang0("java").Versions, v) {
			d.Version = v // Railpack doesn't read toolchains: Gradle would fail with another JDK
		}
		d.Framework = "jar"
		if strings.Contains(pl.Deploy.StartCommand, "-Dserver.port") { // Railpack's Spring Boot detection
			d.Framework = "spring-boot"
		}
	}
	lang, preset, _ := Find(d.Language, d.Framework)
	s := Stack{Language: d.Language, Framework: d.Framework, Version: d.Version}
	switch {
	case d.Framework == "": // language known, framework not: custom commands
		s.Framework = "custom"
		if lang.TrustStart {
			s.StartCommand = pl.Deploy.StartCommand
		}
	case lang.TrustStart && pl.Deploy.StartCommand != "" && !preset.Static:
		// Railpack's own start command (e.g. Django's app module) beats the
		// generic preset.
		s.StartCommand = pl.Deploy.StartCommand
	}
	if d.Framework == "static" && staticRoot != "" {
		s.PublishDir = staticRoot
	}
	if preset.DocRoot { // the folder with the front controller
		for _, dir := range []string{"public", "web"} {
			if isFile(files, dir+"/index.php") {
				s.PublishDir = dir
				break
			}
		}
	}
	if err := s.Normalize(); err != nil && lang.ServedBy == "" { // e.g. custom without a start command: let the student fill it in
		s.Framework, s.StartCommand = "custom", pl.Deploy.StartCommand
	}
	d.Suggested = &s
	label := labelOf(d.Language, s.Framework)
	if d.Framework == "" {
		d.Message = fmt.Sprintf("Detected %s, but not the framework. Check the commands below.", label)
	} else {
		d.Message = "Detected " + label + ". Check it below, change it if it's wrong, then deploy."
	}
	return d
}

// noFiles is an empty folder.
type noFiles struct{}

func (noFiles) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// isFile reports whether name is a regular file of fsys.
func isFile(fsys fs.FS, name string) bool {
	fi, err := fs.Stat(fsys, name)
	return err == nil && fi.Mode().IsRegular()
}

// lang0 returns a catalog language by ID.
func lang0(id string) Language {
	l, _, _ := Find(id, "")
	return l
}

// Java versions declared by Maven (java.version, maven.compiler.release or
// source) and Gradle (toolchain languageVersion, sourceCompatibility).
var (
	mavenJava  = regexp.MustCompile(`<(?:java\.version|maven\.compiler\.(?:release|source))>\s*(?:1\.)?(\d+)\s*<`)
	gradleJava = regexp.MustCompile(`(?:JavaLanguageVersion\.of\(\s*|sourceCompatibility\s*=\s*(?:JavaVersion\.VERSION_|['"]))(\d+)`)
)

// javaVersion reads the Java version a Maven or Gradle build asks for, or "".
func javaVersion(fsys fs.FS) string {
	for name, re := range map[string]*regexp.Regexp{"pom.xml": mavenJava, "build.gradle": gradleJava, "build.gradle.kts": gradleJava} {
		if fi, err := fs.Stat(fsys, name); err != nil || !fi.Mode().IsRegular() || fi.Size() > 1<<20 {
			continue
		}
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			continue
		}
		if m := re.FindSubmatch(b); m != nil {
			return string(m[1])
		}
	}
	return ""
}

func labelOf(language, framework string) string {
	l, p, ok := Find(language, framework)
	if !ok {
		return l.Label
	}
	if l.ID == "static" {
		return l.Label
	}
	return l.Label + " · " + p.Label
}

// majorMinor turns "3.13.18" into "3.13" (or "22.23.4" into "22") when that
// version is selectable, so the review form preselects it.
func majorMinor(v string, allowed []string) string {
	for _, a := range allowed {
		if v == a || strings.HasPrefix(v, a+".") {
			return a
		}
	}
	return ""
}
