package report

import (
	"context"
	"embed"
	"hr-sorter/internal/domain/dto"
	"io"
)

//go:embed assets/*.ttf
var fontAssets embed.FS

// PDFReportService implementation using maroto/v2
type PDFReportService struct {
	// fonts will be loaded from fontAssets
}

func NewPDFReportService() *PDFReportService {
	return &PDFReportService{}
}

func (s *PDFReportService) CreateReport(ctx context.Context, data dto.HHKPI) (io.ReadCloser, error) {
	// 1. Load embedded fonts
	// 2. Initialize maroto/v2 instance
	// 3. Construct header (HR-SORTER Logo, Report Date, Account Name)
	// 4. Construct KPI blocks (Response Rate, Hire Rate)
	// 5. Construct visual recruitment funnel (Total -> Screening -> Tech -> Offer -> Accepted)
	// 6. Generate footer (Page X of Y)
	panic("implement me with maroto/v2")
}
