// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package stacks

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNormalizeFillsPresetDefaults(t *testing.T) {
	s := Stack{Language: "Python", Framework: "FastAPI", Version: "3.12"}
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	if s.Language != "python" || s.Framework != "fastapi" || s.StartCommand != "uvicorn main:app --host 0.0.0.0 --port $PORT" || s.BuildCommand != "pip install uvicorn" {
		t.Fatalf("defaults not applied: %+v", s)
	}
	edited := Stack{Language: "python", Framework: "fastapi", StartCommand: "uvicorn api.server:app --host 0.0.0.0 --port $PORT"}
	if err := edited.Normalize(); err != nil || edited.StartCommand != "uvicorn api.server:app --host 0.0.0.0 --port $PORT" {
		t.Fatalf("student's command must be kept: %+v %v", edited, err)
	}
}

func TestNormalizeStatic(t *testing.T) {
	s := Stack{Language: "static", Framework: "static", PublishDir: "/public/"}
	if err := s.Normalize(); err != nil || s.PublishDir != "public" {
		t.Fatalf("got %+v %v", s, err)
	}
	def := Stack{Language: "static", Framework: "static"}
	if err := def.Normalize(); err != nil || def.PublishDir != "." {
		t.Fatalf("default publish dir: %+v %v", def, err)
	}
	vite := Stack{Language: "node", Framework: "vite"}
	if err := vite.Normalize(); err != nil || vite.PublishDir != "dist" || vite.StartCommand != "" {
		t.Fatalf("vite: %+v %v", vite, err)
	}
	nonStatic := Stack{Language: "node", Framework: "node-server", PublishDir: "dist"}
	if err := nonStatic.Normalize(); err != nil || nonStatic.PublishDir != "" {
		t.Fatalf("publish dir must be dropped for servers: %+v %v", nonStatic, err)
	}
}

func TestNormalizeRejects(t *testing.T) {
	bad := map[string]Stack{
		"unknown language":        {Language: "cobol", Framework: "x"},
		"unknown framework":       {Language: "python", Framework: "rails"},
		"custom without run":      {Language: "node", Framework: "custom", BuildCommand: "npm ci"},
		"unsupported version":     {Language: "python", Framework: "flask", Version: "2.7"},
		"version on static":       {Language: "static", Framework: "static", Version: "1"},
		"static with run command": {Language: "static", Framework: "static", StartCommand: "python -m http.server"},
		"publish dir escapes":     {Language: "static", Framework: "static", PublishDir: "../etc"},
		"publish dir option-like": {Language: "static", Framework: "static", PublishDir: "-rf"},
		"publish dir odd chars":   {Language: "static", Framework: "static", PublishDir: "pub lic;rm"},
		"multi-line command":      {Language: "node", Framework: "custom", StartCommand: "a\nb"},
		"too long command":        {Language: "node", Framework: "custom", StartCommand: strings.Repeat("x", MaxCommandLen+1)},
	}
	for name, s := range bad {
		if err := s.Normalize(); err == nil {
			t.Errorf("%s: accepted %+v", name, s)
		}
	}
}

func TestRailpackOptions(t *testing.T) {
	static := Stack{Language: "static", Framework: "static", PublishDir: "site"}
	o := static.Railpack()
	if string(o.Config) != `{"provider":"staticfile"}` || o.Env["RAILPACK_STATIC_FILE_ROOT"] != "site" || o.RequireStart || o.StartCommand != "" {
		t.Fatalf("static: %+v config=%s", o, o.Config)
	}
	py := Stack{Language: "python", Framework: "fastapi", Version: "3.12", BuildCommand: "pip install uvicorn", StartCommand: "uvicorn main:app"}
	o = py.Railpack()
	var cfg map[string]any
	json.Unmarshal(o.Config, &cfg) //nolint:errcheck
	if cfg["provider"] != "python" || cfg["packages"].(map[string]any)["python"] != "3.12" || !o.RequireStart || o.StartCommand != "uvicorn main:app" {
		t.Fatalf("python: %+v config=%s", o, o.Config)
	}
	spa := Stack{Language: "node", Framework: "vite", PublishDir: "dist", BuildCommand: "npm run build"}
	o = spa.Railpack()
	if o.Env["RAILPACK_SPA_OUTPUT_DIR"] != "dist" || o.StartCommand != "" || o.RequireStart {
		t.Fatalf("vite: %+v", o)
	}
}

// Real Railpack 0.43.0 outputs captured from sample repositories.
const (
	infoNode   = `{"resolvedPackages":{"node":{"resolvedVersion":"22.23.4"}},"metadata":{"nodePackageManager":"npm","nodeRuntime":"node","providers":"node"},"detectedProviders":["node"],"logs":[{"Level":"warn","Msg":"package-lock.json detected, assuming npm"}],"success":true}`
	infoStatic = `{"resolvedPackages":{"caddy":{"resolvedVersion":"2.11.8"}},"metadata":{"providers":"staticfile"},"detectedProviders":["staticfile"],"logs":[{"Level":"info","Msg":"Detected Staticfile"},{"Level":"info","Msg":"Using staticfile root dir: ."}],"success":true}`
	infoFlask  = `{"resolvedPackages":{"python":{"resolvedVersion":"3.13.18"}},"metadata":{"providers":"python","pythonPackageManager":"pip","pythonRuntime":"flask"},"detectedProviders":["python"],"success":true}`
	infoNone   = `{"detectedProviders":[],"logs":[{"Level":"error","Msg":"no provider matched"}],"success":false}`
	infoRuby   = `{"detectedProviders":["ruby"],"metadata":{"providers":"ruby"},"success":true}`
)

func TestDetect(t *testing.T) {
	cases := []struct {
		name, info, plan                         string
		detected                                 bool
		lang, framework, version, start, publish string
	}{
		{"node", infoNode, `{"deploy":{"startCommand":"npm run start"}}`, true, "node", "node-server", "22", "npm run start", ""},
		{"static", infoStatic, `{"deploy":{}}`, true, "static", "static", "", "", "."},
		{"flask without gunicorn", infoFlask, `{"deploy":{}}`, true, "python", "flask", "3.13", "gunicorn --bind 0.0.0.0:$PORT app:app", ""},
		{"nothing", infoNone, ``, false, "", "", "", "", ""},
		{"unsupported language", infoRuby, ``, false, "", "", "", "", ""},
	}
	for _, c := range cases {
		d := Detect([]byte(c.info), []byte(c.plan), nil)
		if d.Detected != c.detected || d.Language != c.lang || d.Framework != c.framework || d.Version != c.version || d.Message == "" {
			t.Errorf("%s: got %+v", c.name, d)
			continue
		}
		if !c.detected {
			if d.Suggested != nil {
				t.Errorf("%s: no suggestion expected", c.name)
			}
			continue
		}
		if d.Suggested == nil || d.Suggested.StartCommand != c.start || d.Suggested.PublishDir != c.publish {
			t.Errorf("%s: suggestion %+v", c.name, d.Suggested)
		}
	}
	// Railpack's own start command (e.g. Django's module) beats the generic preset.
	django := Detect([]byte(`{"detectedProviders":["python"],"metadata":{"pythonRuntime":"django"}}`),
		[]byte(`{"deploy":{"startCommand":"python manage.py migrate && gunicorn shop.wsgi:application"}}`), nil)
	if django.Framework != "django" || django.Suggested.StartCommand != "python manage.py migrate && gunicorn shop.wsgi:application" {
		t.Errorf("django: %+v %+v", django, django.Suggested)
	}
}

func TestCleanRootDir(t *testing.T) {
	good := map[string]string{"": "", ".": "", "/": "", "./kaat.cm/": "kaat.cm", "apps/web": "apps/web", " frontend ": "frontend"}
	for in, want := range good {
		if got, err := CleanRootDir(in); err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"..", "a/../../etc", "-rf", "a b", "a;b", "$(x)"} {
		if _, err := CleanRootDir(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	s := Stack{Language: "node", Framework: "vite", RootDir: "./kaat.cm"}
	if err := s.Normalize(); err != nil || s.RootDir != "kaat.cm" || s.PublishDir != "dist" {
		t.Errorf("vite in a subfolder: %+v %v", s, err)
	}
}

// fixture reads a Railpack 0.43.0 info output captured on the test VM
// (testdata/*.json, which embed the plan's start command) and returns the
// info and plan parts.
func fixture(t *testing.T, name string) (info, plan []byte) {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Plan json.RawMessage `json:"plan"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return raw, v.Plan
}

// FodyCloud's Java start commands (Railpack's Gradle one misses
// single-project builds).
const (
	springStart = "java $JAVA_OPTS -Dserver.port=$PORT -jar " + javaJar
	jarStart    = "java $JAVA_OPTS -jar " + javaJar
)

func TestDetectNewStacks(t *testing.T) {
	cases := []struct {
		fixture, lang, framework, version, start, publish string
		files                                             fstest.MapFS
	}{
		// Procfile commands written for other platforms are ignored.
		{"go-procfile", "go", "go-server", "1.27", "./out", "", nil},
		// PHP's version comes from composer.json, through the FrankenPHP image.
		{"php", "php", "php", "", "", "web", fstest.MapFS{"web/index.php": {}}},
		{"php", "php", "php", "", "", ".", nil},
		{"php", "php", "php", "", "", ".", fstest.MapFS{"web/index.php": {Mode: fs.ModeSymlink}}},
		{"laravel", "php", "laravel", "", "", "", fstest.MapFS{"public/index.php": {}}},
		{"bun", "node", "bun", "", "bun run start", "", nil},
		{"spring-gradle", "java", "spring-boot", "21", springStart, "", nil},
		// Railpack doesn't read Gradle toolchains: the build file decides.
		{"spring-gradle", "java", "spring-boot", "17", springStart, "",
			fstest.MapFS{"build.gradle": {Data: []byte("java {\n  toolchain {\n    languageVersion = JavaLanguageVersion.of(17)\n  }\n}\n")}}},
		{"java-maven", "java", "jar", "21", jarStart, "", nil},
		{"java-maven", "java", "jar", "25", jarStart, "",
			fstest.MapFS{"pom.xml": {Data: []byte("<properties><java.version>25</java.version></properties>")}}},
		// An unsupported version keeps Railpack's choice.
		{"java-maven", "java", "jar", "21", jarStart, "",
			fstest.MapFS{"pom.xml": {Data: []byte("<maven.compiler.source>1.8</maven.compiler.source>")}}},
	}
	for i, c := range cases {
		info, plan := fixture(t, c.fixture)
		var files fs.FS
		if c.files != nil {
			files = c.files
		}
		d := Detect(info, plan, files)
		name := fmt.Sprintf("%s #%d", c.fixture, i)
		if !d.Detected || d.Language != c.lang || d.Framework != c.framework || d.Version != c.version {
			t.Errorf("%s: got %+v", name, d)
			continue
		}
		if s := d.Suggested; s == nil || s.Framework != c.framework || s.StartCommand != c.start || s.PublishDir != c.publish {
			t.Errorf("%s: suggestion %+v", name, d.Suggested)
		}
	}
}

func TestNewStacksNormalize(t *testing.T) {
	bad := map[string]Stack{
		"PHP with a run command":     {Language: "php", Framework: "php", StartCommand: "php -S 0.0.0.0:$PORT"},
		"Laravel with a run command": {Language: "php", Framework: "laravel", StartCommand: "php artisan serve"},
		"PHP root escapes":           {Language: "php", Framework: "php", PublishDir: "../x"},
		"old Go":                     {Language: "go", Framework: "go-server", Version: "1.10"},
		"PHP version":                {Language: "php", Framework: "php", Version: "8.4"},
		"Java 8":                     {Language: "java", Framework: "spring-boot", Version: "8"},
	}
	for name, s := range bad {
		if err := s.Normalize(); err == nil {
			t.Errorf("%s: accepted %+v", name, s)
		}
	}
	lar := Stack{Language: "php", Framework: "laravel", PublishDir: "public"}
	if err := lar.Normalize(); err != nil || lar.PublishDir != "" {
		t.Errorf("laravel: %+v %v", lar, err)
	}
	php := Stack{Language: "php", Framework: "php"}
	if err := php.Normalize(); err != nil || php.PublishDir != "." {
		t.Errorf("php default root: %+v %v", php, err)
	}
	spring := Stack{Language: "java", Framework: "spring-boot"}
	if err := spring.Normalize(); err != nil || spring.StartCommand != springStart {
		t.Errorf("spring runs the built jar with the port: %+v %v", spring, err)
	}
}

func TestNewStacksRailpack(t *testing.T) {
	config := func(o Options) map[string]any {
		var cfg map[string]any
		if err := json.Unmarshal(o.Config, &cfg); err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	php := Stack{Language: "php", Framework: "php", PublishDir: "web"}
	o := php.Railpack()
	if o.StartCommand != "/start-container.sh" || !o.RequireStart || o.Env["RAILPACK_PHP_ROOT_DIR"] != "/app/web" ||
		config(o)["packages"] != nil {
		t.Errorf("php: %+v %s", o, o.Config)
	}
	if o := (Stack{Language: "php", Framework: "php", PublishDir: "."}).Railpack(); o.Env["RAILPACK_PHP_ROOT_DIR"] != "/app" {
		t.Errorf("php at the root: %+v", o.Env)
	}
	if o := (Stack{Language: "php", Framework: "laravel"}).Railpack(); o.StartCommand != "/start-container.sh" || o.Env["RAILPACK_PHP_ROOT_DIR"] != "" {
		t.Errorf("laravel: %+v", o)
	}
	if o := (Stack{Language: "go", Framework: "go-server", StartCommand: "./out"}).Railpack(); o.StartCommand != "./out" || config(o)["provider"] != "golang" {
		t.Errorf("go: %+v %s", o, o.Config)
	}
	if o := (Stack{Language: "node", Framework: "bun", StartCommand: "bun run start"}).Railpack(); config(o)["packages"].(map[string]any)["bun"] != "latest" {
		t.Errorf("bun: %s", o.Config)
	}
	o = (Stack{Language: "java", Framework: "spring-boot", Version: "21", StartCommand: springStart}).Railpack()
	vars := config(o)["deploy"].(map[string]any)["variables"].(map[string]any)
	if vars["JAVA_OPTS"] != "-XX:MaxRAMPercentage=75.0" || o.StartCommand != springStart || !o.RequireStart {
		t.Errorf("java: %+v %s", o, o.Config)
	}
}
