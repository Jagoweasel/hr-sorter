package web

import (
	"context"
	"hr-sorter/internal/i18n"
	"testing"
)

func TestNewHandler(t *testing.T) {
	ls, _ := i18n.NewLocalizationService()
	tm, _ := NewTemplateManager(ls)

	h := NewHandler(context.Background(), nil, nil, nil, tm, ls, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
}
