package report

import (
	"context"
	"embed"
	"fmt"
	"hr-sorter/internal/domain"
	"hr-sorter/internal/models"
	"os"
	"runtime"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/line"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/core/entity"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

//go:embed assets/fonts/*.ttf
var fonts embed.FS

type reporter struct {
}

func NewReporter() domain.Reporter {
	return &reporter{}
}

func (r *reporter) GeneratePDF(ctx context.Context, data *models.ReportData) ([]byte, error) {
	regular, _ := fonts.ReadFile("assets/fonts/Inter-Regular.ttf")
	bold, _ := fonts.ReadFile("assets/fonts/Inter-Bold.ttf")

	builder := config.NewBuilder().
		WithTitle("HR-SORTER Report", true).
		WithAuthor("HR-SORTER", true)

	// Attempt to load custom fonts from embed or system
	if len(regular) > 0 && len(bold) > 0 {
		builder.WithCustomFonts([]*entity.CustomFont{
			{Family: "Inter", Style: fontstyle.Normal, Bytes: regular},
			{Family: "Inter", Style: fontstyle.Bold, Bytes: bold},
		}).WithDefaultFont(&props.Font{Family: "Inter"})
	} else {
		// Fallback to system fonts for Cyrillic support if embed fonts are missing/empty
		systemFont := ""
		if runtime.GOOS == "windows" {
			systemFont = os.Getenv("WINDIR") + "\\Fonts\\arial.ttf"
		} else {
			// Common Linux path
			systemFont = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
		}

		if _, err := os.Stat(systemFont); err == nil {
			fontBytes, _ := os.ReadFile(systemFont)
			builder.WithCustomFonts([]*entity.CustomFont{
				{Family: "System", Style: fontstyle.Normal, Bytes: fontBytes},
				{Family: "System", Style: fontstyle.Bold, Bytes: fontBytes},
			}).WithDefaultFont(&props.Font{Family: "System"})
		}
	}

	cfg := builder.Build()

	m := maroto.New(cfg)

	// Header
	r.buildHeader(m, data)

	// KPI Summary
	r.buildKPI(m, data)

	// Funnel
	r.buildFunnel(m, data)

	// Sequences Table
	r.buildTable(m, data)

	// Footer (automatic via Page Numbering)
	m.RegisterFooter(
		row.New(10).Add(
			col.New(12).Add(
				text.New("Page {current} of {total}", props.Text{
					Size:  8,
					Align: align.Right,
				}),
			),
		),
	)

	// Generate
	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	return doc.GetBytes(), nil
}

func (r *reporter) buildHeader(m core.Maroto, data *models.ReportData) {
	m.AddRow(20,
		col.New(4).Add(
			text.New("HR-SORTER", props.Text{
				Size:  16,
				Style: fontstyle.Bold,
				Align: align.Left,
			}),
		),
		col.New(4).Add(
			text.New(fmt.Sprintf("Account: %s", data.AccountName), props.Text{
				Size:  10,
				Align: align.Center,
			}),
		),
		col.New(4).Add(
			text.New(fmt.Sprintf("Date: %s", data.ReportDate.Format("02.01.2006")), props.Text{
				Size:  10,
				Align: align.Right,
			}),
		),
	)
	m.AddRow(5, col.New(12).Add(line.New(props.Line{Thickness: 0.5})))
}

func (r *reporter) buildKPI(m core.Maroto, data *models.ReportData) {
	m.AddRow(10)
	m.AddRow(10, col.New(12).Add(text.New("KPI Summary", props.Text{Size: 14, Style: fontstyle.Bold})))
	m.AddRow(15,
		col.New(4).Add(
			text.New(fmt.Sprintf("Total Sequences: %d", data.KPI.TotalSequences), props.Text{Size: 10}),
		),
		col.New(4).Add(
			text.New(fmt.Sprintf("Response Rate: %.1f%%", data.KPI.ResponseRate), props.Text{Size: 10}),
		),
		col.New(4).Add(
			text.New(fmt.Sprintf("Hire Rate: %.1f%%", data.KPI.HireRate), props.Text{Size: 10}),
		),
	)
}

func (r *reporter) buildFunnel(m core.Maroto, data *models.ReportData) {
	m.AddRow(10)
	m.AddRow(10, col.New(12).Add(text.New("Recruitment Funnel", props.Text{Size: 14, Style: fontstyle.Bold})))

	m.AddRow(15,
		col.New(2).Add(text.New(fmt.Sprintf("Initial: %d", data.Funnel.Initial), props.Text{Size: 9, Align: align.Center})),
		col.New(1).Add(text.New("->", props.Text{Size: 9, Align: align.Center})),
		col.New(2).Add(text.New(fmt.Sprintf("Screening: %d", data.Funnel.Screening), props.Text{Size: 9, Align: align.Center})),
		col.New(1).Add(text.New("->", props.Text{Size: 9, Align: align.Center})),
		col.New(2).Add(text.New(fmt.Sprintf("Tech: %d", data.Funnel.Tech), props.Text{Size: 9, Align: align.Center})),
		col.New(1).Add(text.New("->", props.Text{Size: 9, Align: align.Center})),
		col.New(3).Add(text.New(fmt.Sprintf("Offer: %d (Acc: %d)", data.Funnel.Offer, data.Funnel.Accepted), props.Text{Size: 9, Align: align.Center})),
	)
}

func (r *reporter) buildTable(m core.Maroto, data *models.ReportData) {
	m.AddRow(10)
	m.AddRow(10, col.New(12).Add(text.New("Detailed Sequences", props.Text{Size: 14, Style: fontstyle.Bold})))

	// Table Header
	m.AddRow(10,
		col.New(3).Add(text.New("Company", props.Text{Style: fontstyle.Bold, Size: 10})),
		col.New(4).Add(text.New("Vacancy", props.Text{Style: fontstyle.Bold, Size: 10})),
		col.New(2).Add(text.New("Category", props.Text{Style: fontstyle.Bold, Size: 10})),
		col.New(3).Add(text.New("Status", props.Text{Style: fontstyle.Bold, Size: 10})),
	)
	m.AddRow(2, col.New(12).Add(line.New(props.Line{Thickness: 0.2})))

	for _, seq := range data.Sequences {
		category := "N/A"
		if seq.Category != nil {
			category = *seq.Category
		}
		m.AddRow(8,
			col.New(3).Add(text.New(seq.CompanyName, props.Text{Size: 9})),
			col.New(4).Add(text.New(seq.VacancyName, props.Text{Size: 9})),
			col.New(2).Add(text.New(category, props.Text{Size: 9})),
			col.New(3).Add(text.New(seq.Status, props.Text{Size: 9})),
		)
	}
}
