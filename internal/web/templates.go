package web

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TemplateManager struct {
	templates map[string]*template.Template
}

func NewTemplateManager() (*TemplateManager, error) {
	tm := &TemplateManager{
		templates: make(map[string]*template.Template),
	}

	funcMap := template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"formatTime": func(t string) string {
			parsed, err := time.Parse("2006-01-02 15:04:05", t)
			if err == nil {
				return parsed.Format("Jan 02, 15:04")
			}
			parsed, err = time.Parse(time.RFC3339, t)
			if err == nil {
				return parsed.Format("Jan 02, 15:04")
			}
			return t
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"lastStage": func(history interface{}) string {
			return "None"
		},
		"slice": func(s string, start, end int) string {
			if len(s) < end {
				if len(s) < start {
					return ""
				}
				return s[start:]
			}
			return s[start:end]
		},
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
	}

	layoutPath := filepath.Join("templates", "layout.html")

	err := filepath.Walk("templates", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		fileName := info.Name()
		if fileName == "layout.html" {
			return nil
		}

		// Get path relative to 'templates/'
		relPath, err := filepath.Rel("templates", path)
		if err != nil {
			return err
		}
		// Normalize to forward slashes for the map key
		key := filepath.ToSlash(relPath)

		var t *template.Template
		if !strings.Contains(key, "/") && strings.HasSuffix(key, ".html") {
			// Main page: Parse with layout
			t, err = template.New(fileName).Funcs(funcMap).ParseFiles(layoutPath, path)
		} else {
			// Fragment or JSON: Parse as standalone
			t, err = template.New(fileName).Funcs(funcMap).ParseFiles(path)
		}

		if err != nil {
			return fmt.Errorf("error parsing %s: %v", path, err)
		}

		tm.templates[key] = t
		return nil
	})

	if err != nil {
		return nil, err
	}

	return tm, nil
}

func (tm *TemplateManager) Render(w io.Writer, name string, data interface{}) error {
	t, ok := tm.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}

	// If it's a main page (no slash), use ExecuteTemplate with layout.html
	if !strings.Contains(name, "/") {
		return t.ExecuteTemplate(w, "layout.html", data)
	}

	return t.Execute(w, data)
}

func (tm *TemplateManager) RenderWithStatus(w http.ResponseWriter, name string, status int, data interface{}) {
	// Set status before rendering
	w.WriteHeader(status)
	if err := tm.Render(w, name, data); err != nil {
		log.Printf("Template error: %v", err)
		// Note: header is already sent, so this Error call might not behave as expected
		// but it's better than silence. In production, we'd handle this differently.
		fmt.Fprintf(w, "Error rendering template: %v", err)
	}
}
