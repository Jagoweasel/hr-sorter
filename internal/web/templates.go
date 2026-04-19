package web

import (
	"fmt"
	"github.com/gorilla/csrf"
	"hr-sorter/internal/i18n"
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
	i18n      *i18n.LocalizationService
}

func NewTemplateManager(ls *i18n.LocalizationService) (*TemplateManager, error) {
	tm := &TemplateManager{
		templates: make(map[string]*template.Template),
		i18n:      ls,
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
		"lower": func(s interface{}) string {
			return strings.ToLower(fmt.Sprint(s))
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
		"split": func(s, sep string) []string {
			if s == "" {
				return nil
			}
			return strings.Split(s, sep)
		},
		"safe": func(s string) template.HTML {
			return template.HTML(s)
		},
		"T": func(locale, key string, args ...interface{}) string {
			return ls.Translate(key, locale, args...)
		},
		"Tr": func(key string, args ...interface{}) string {
			return "" // Placeholder
		},
		"Locale": func() string {
			return "" // Placeholder
		},
		"csrfField": func() template.HTML {
			return "" // Placeholder
		},
		"csrfToken": func() string {
			return "" // Placeholder
		},
	}

	layoutPath := filepath.Join("templates", "layout.html")

	// First, parse all fragments into a common pool
	fragments, err := filepath.Glob("templates/fragments/*.html")
	if err != nil {
		return nil, err
	}
	modalFragments, _ := filepath.Glob("templates/fragments/modals/*.html")
	pipelineFragments, _ := filepath.Glob("templates/fragments/pipeline/*.html")

	var allFragments []string
	allFragments = append(allFragments, fragments...)
	allFragments = append(allFragments, modalFragments...)
	allFragments = append(allFragments, pipelineFragments...)

	err = filepath.Walk("templates", func(path string, info os.FileInfo, err error) error {
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
			// Main page: Parse with layout and ALL fragments
			files := append([]string{layoutPath, path}, allFragments...)
			t, err = template.New("layout.html").Funcs(funcMap).ParseFiles(files...)
		} else {
			// Fragment or JSON: Parse with ALL other fragments so they can include each other
			files := append([]string{path}, allFragments...)
			t, err = template.New(fileName).Funcs(funcMap).ParseFiles(files...)
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

func (tm *TemplateManager) Render(w io.Writer, r *http.Request, name string, data interface{}, locale string) error {
	t, ok := tm.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}

	// Clone to safely add request-specific Funcs
	t, err := t.Clone()
	if err != nil {
		return err
	}

	t.Funcs(template.FuncMap{
		"Tr": func(key string, args ...interface{}) string {
			return tm.i18n.Translate(key, locale, args...)
		},
		"Locale": func() string {
			return locale
		},
		"csrfField": func() template.HTML {
			return csrf.TemplateField(r)
		},
		"csrfToken": func() string {
			return csrf.Token(r)
		},
	})

	// If it's a main page (no slash), use ExecuteTemplate with layout.html
	if !strings.Contains(name, "/") {
		if r.Header.Get("HX-Request") != "" {
			return t.ExecuteTemplate(w, "content", data)
		}
		return t.ExecuteTemplate(w, "layout.html", data)
	}

	return t.Execute(w, data)
}

func (tm *TemplateManager) RenderWithStatus(w http.ResponseWriter, r *http.Request, name string, status int, data interface{}, locale string) {
	// Set status and content type before rendering
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := tm.Render(w, r, name, data, locale); err != nil {
		log.Printf("Template error: %v", err)
		fmt.Fprintf(w, "Error rendering template: %v", err)
	}
}
