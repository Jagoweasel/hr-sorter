package report

import (
	"context"
	"embed"
	"fmt"
	"hr-sorter/internal/domain"
	"hr-sorter/internal/models"
	"math"
	"os"
	"runtime"
	"strings"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/line"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
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
		m.AddRow(10) // Spacer
		m.AddRow(5, col.New(12).Add(line.New(props.Line{Thickness: 1, Color: &props.Color{Red: 200, Green: 200, Blue: 200}})))
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

func (r *reporter) getStatusColor(status string) *props.Color {
	switch strings.ToLower(status) {
	case "accepted":
		return &props.Color{Red: 187, Green: 247, Blue: 208} // bg-green-100
	case "rejected":
		return &props.Color{Red: 254, Green: 202, Blue: 202} // bg-red-100
	case "offer":
		return &props.Color{Red: 254, Green: 249, Blue: 195} // bg-yellow-100
	case "final":
		return &props.Color{Red: 252, Green: 231, Blue: 243} // bg-pink-100
	case "tech":
		return &props.Color{Red: 243, Green: 232, Blue: 255} // bg-purple-100
	case "screening":
		return &props.Color{Red: 224, Green: 231, Blue: 255} // bg-indigo-100
	default:
		return &props.Color{Red: 219, Green: 234, Blue: 254} // bg-blue-100
	}
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

	// KPI Summary
	m.AddRow(10, col.New(12).Add(text.New("KPI SUMMARY", props.Text{Size: 10, Style: fontstyle.Bold, Color: &props.Color{Red: 100, Green: 100, Blue: 100}})))

	m.AddRow(15,
		col.New(4).Add(
			text.New("Applied", props.Text{Size: 8}),
			text.New(fmt.Sprintf("%d", kpi.TotalApplied), props.Text{Size: 12, Style: fontstyle.Bold, Top: 4}),
		),
		col.New(4).Add(
			text.New("Sequences", props.Text{Size: 8}),
			text.New(fmt.Sprintf("%d", kpi.TotalSequences), props.Text{Size: 12, Style: fontstyle.Bold, Top: 4}),
		),
		col.New(4).Add(
			text.New("Conv. Rate", props.Text{Size: 8}),
			text.New(fmt.Sprintf("%.1f%%", kpi.ResponseRate), props.Text{Size: 12, Style: fontstyle.Bold, Top: 4}),
		),
	)

	// Recruitment Funnel
	m.AddRow(10, col.New(12).Add(text.New("RECRUITMENT FUNNEL", props.Text{Size: 10, Style: fontstyle.Bold, Color: &props.Color{Red: 100, Green: 100, Blue: 100}})))
	m.AddRow(5)

	maxVal := funnel.Applied
	if maxVal == 0 {
		maxVal = 1
	}

	steps := []struct {
		label string
		count int
		color *props.Color
	}{
		{"Applied", funnel.Applied, &props.Color{Red: 249, Green: 115, Blue: 22}},     // Orange
		{"Initial", funnel.Initial, &props.Color{Red: 59, Green: 130, Blue: 246}},     // Blue
		{"Interviews", funnel.Screening, &props.Color{Red: 79, Green: 70, Blue: 229}}, // Indigo
		{"Technical", funnel.Tech, &props.Color{Red: 147, Green: 51, Blue: 234}},      // Purple
		{"Offers", funnel.Offer, &props.Color{Red: 234, Green: 179, Blue: 8}},         // Yellow
	}

	for _, step := range steps {
		// Use logarithmic scale to make small numbers more visible relative to large ones
		// but still maintain the "funnel" feeling
		// Log(count+1) / Log(maxVal+1) gives a nice distribution
		perc := 0.0
		if step.count > 0 {
			perc = (math.Log10(float64(step.count) + 1)) / (math.Log10(float64(maxVal) + 1)) * 100
		}

		if step.count > 0 && perc < 10 {
			perc = 10 // Minimum visibility for any data point
		}
		if perc > 100 {
			perc = 100
		}

		// Use 8 columns for the bar instead of 10 to fit in 12-column grid (2 label + 8 bar + 2 count)
		coloredCols := int(perc * 8 / 100)
		if coloredCols == 0 && step.count > 0 {
			coloredCols = 1
		}

		// If count is 0, we don't draw the colored part to avoid Maroto v2 layout weirdness
		if coloredCols > 0 {
			m.AddRow(8,
				col.New(2).Add(text.New(step.label, props.Text{Size: 8, Style: fontstyle.Bold})),
				col.New(coloredCols).WithStyle(&props.Cell{BackgroundColor: step.color}).Add(
					text.New(fmt.Sprintf("%d", step.count), props.Text{Size: 7, Align: align.Center, Color: &props.Color{Red: 255, Green: 255, Blue: 255}}),
				),
				col.New(8-coloredCols).WithStyle(&props.Cell{BackgroundColor: &props.Color{Red: 243, Green: 244, Blue: 246}}),
				col.New(2).Add(text.New(fmt.Sprintf("%d", step.count), props.Text{Size: 8, Align: align.Right})),
			)
		} else {
			m.AddRow(8,
				col.New(2).Add(text.New(step.label, props.Text{Size: 8, Style: fontstyle.Bold})),
				col.New(8).WithStyle(&props.Cell{BackgroundColor: &props.Color{Red: 243, Green: 244, Blue: 246}}),
				col.New(2).Add(text.New(fmt.Sprintf("%d", step.count), props.Text{Size: 8, Align: align.Right})),
			)
		}
		m.AddRow(2)
	}

	// Details Table
	m.AddRow(10, col.New(12).Add(text.New("DETAILED LIST", props.Text{Size: 10, Style: fontstyle.Bold, Color: &props.Color{Red: 100, Green: 100, Blue: 100}})))

	headerColor := &props.Color{Red: 240, Green: 240, Blue: 240}
	m.AddRow(10,
		col.New(3).Add(text.New("Company", props.Text{Size: 8, Style: fontstyle.Bold})).WithStyle(&props.Cell{BackgroundColor: headerColor, BorderType: border.Full, BorderThickness: 0.1}),
		col.New(4).Add(text.New("Vacancy", props.Text{Size: 8, Style: fontstyle.Bold})).WithStyle(&props.Cell{BackgroundColor: headerColor, BorderType: border.Full, BorderThickness: 0.1}),
		col.New(2).Add(text.New("Last Stage", props.Text{Size: 8, Style: fontstyle.Bold})).WithStyle(&props.Cell{BackgroundColor: headerColor, BorderType: border.Full, BorderThickness: 0.1}),
		col.New(3).Add(text.New("Status", props.Text{Size: 8, Style: fontstyle.Bold})).WithStyle(&props.Cell{BackgroundColor: headerColor, BorderType: border.Full, BorderThickness: 0.1}),
	)

	for _, seq := range sequences {
		lastStage := "Initial"
		if seq.Category != nil {
			lastStage = *seq.Category
		}

		m.AddRow(18,
			col.New(3).Add(text.New(seq.CompanyName, props.Text{Size: 8, Style: fontstyle.Bold})).WithStyle(&props.Cell{BorderType: border.Full, BorderThickness: 0.1}),
			col.New(4).Add(text.New(seq.VacancyName, props.Text{Size: 7})).WithStyle(&props.Cell{BorderType: border.Full, BorderThickness: 0.1}),
			col.New(2).Add(text.New(lastStage, props.Text{Size: 7, Align: align.Center})).WithStyle(&props.Cell{BorderType: border.Full, BorderThickness: 0.1}),
			col.New(3).Add(text.New(strings.ToUpper(seq.Status), props.Text{Size: 7, Style: fontstyle.Bold, Align: align.Center})).WithStyle(&props.Cell{
				BackgroundColor: r.getStatusColor(seq.Status),
				BorderType:      border.Full,
				BorderThickness: 0.1,
			}),
		)
	}
}
