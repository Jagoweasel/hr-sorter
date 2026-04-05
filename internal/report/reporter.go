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

	if len(regular) > 0 && len(bold) > 0 {
		builder.WithCustomFonts([]*entity.CustomFont{
			{Family: "Inter", Style: fontstyle.Normal, Bytes: regular},
			{Family: "Inter", Style: fontstyle.Bold, Bytes: bold},
		}).WithDefaultFont(&props.Font{Family: "Inter"})
	} else {
		systemFont := ""
		if runtime.GOOS == "windows" {
			systemFont = os.Getenv("WINDIR") + "\\Fonts\\arial.ttf"
		} else {
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

	// Global Section
	r.buildSection(m, data.AccountName, data.KPI, data.Funnel, data.Sequences, true)

	// Additional Sections per Account
	for _, section := range data.Sections {
		r.buildSection(m, section.AccountName, section.KPI, section.Funnel, section.Sequences, false)
	}

	// Footer
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

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	return doc.GetBytes(), nil
}

func (r *reporter) buildSection(m core.Maroto, title string, kpi models.ReportKPI, funnel models.Funnel, sequences []models.Sequence, isGlobal bool) {
	// Header
	headerSize := 14.0
	if isGlobal {
		headerSize = 18.0
	}

	m.AddRow(20,
		col.New(8).Add(
			text.New(title, props.Text{
				Size:  headerSize,
				Style: fontstyle.Bold,
				Align: align.Left,
			}),
		),
		col.New(4).Add(
			text.New("HR-SORTER", props.Text{
				Size:  10,
				Style: fontstyle.Bold,
				Align: align.Right,
			}),
		),
	)
	m.AddRow(5, col.New(12).Add(line.New(props.Line{Thickness: 0.5})))

	// KPI Summary
	m.AddRow(10)
	m.AddRow(10, col.New(12).Add(text.New("KPI Summary", props.Text{Size: 12, Style: fontstyle.Bold})))
	m.AddRow(15,
		col.New(3).Add(text.New(fmt.Sprintf("Applied: %d", kpi.TotalApplied), props.Text{Size: 9})),
		col.New(3).Add(text.New(fmt.Sprintf("Sequences: %d", kpi.TotalSequences), props.Text{Size: 9})),
		col.New(3).Add(text.New(fmt.Sprintf("Response Rate: %.1f%%", kpi.ResponseRate), props.Text{Size: 9})),
		col.New(3).Add(text.New(fmt.Sprintf("Hire Rate: %.1f%%", kpi.HireRate), props.Text{Size: 9})),
	)

	// Funnel
	m.AddRow(10, col.New(12).Add(text.New("Recruitment Funnel", props.Text{Size: 12, Style: fontstyle.Bold})))
	m.AddRow(15,
		col.New(2).Add(text.New(fmt.Sprintf("App: %d", funnel.Applied), props.Text{Size: 8, Align: align.Center})),
		col.New(2).Add(text.New(fmt.Sprintf("Init: %d", funnel.Initial), props.Text{Size: 8, Align: align.Center})),
		col.New(2).Add(text.New(fmt.Sprintf("Scr: %d", funnel.Screening), props.Text{Size: 8, Align: align.Center})),
		col.New(2).Add(text.New(fmt.Sprintf("Tech: %d", funnel.Tech), props.Text{Size: 8, Align: align.Center})),
		col.New(2).Add(text.New(fmt.Sprintf("Off: %d", funnel.Offer), props.Text{Size: 8, Align: align.Center})),
		col.New(2).Add(text.New(fmt.Sprintf("Acc: %d", funnel.Accepted), props.Text{Size: 8, Align: align.Center})),
	)

	// Sequences Table
	m.AddRow(10, col.New(12).Add(text.New("Details", props.Text{Size: 12, Style: fontstyle.Bold})))
	m.AddRow(10,
		col.New(3).Add(text.New("Company", props.Text{Style: fontstyle.Bold, Size: 9})),
		col.New(4).Add(text.New("Vacancy", props.Text{Style: fontstyle.Bold, Size: 9})),
		col.New(2).Add(text.New("Category", props.Text{Style: fontstyle.Bold, Size: 9})),
		col.New(3).Add(text.New("Status", props.Text{Style: fontstyle.Bold, Size: 9})),
	)
	m.AddRow(2, col.New(12).Add(line.New(props.Line{Thickness: 0.2})))

	for _, seq := range sequences {
		category := "N/A"
		if seq.Category != nil {
			category = *seq.Category
		}
		m.AddRow(8,
			col.New(3).Add(text.New(seq.CompanyName, props.Text{Size: 8})),
			col.New(4).Add(text.New(seq.VacancyName, props.Text{Size: 8})),
			col.New(2).Add(text.New(category, props.Text{Size: 8})),
			col.New(3).Add(text.New(seq.Status, props.Text{Size: 8})),
		)
	}
}
