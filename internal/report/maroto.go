package report

import (
	"bytes"
	"context"
	"fmt"
	"hr-sorter/internal/domain"
	"hr-sorter/internal/models"
	"io"
)

// PDFReportService implementation using maroto/v2
type PDFReportService struct {
	reporter domain.Reporter
}

func NewPDFReportService() *PDFReportService {
	return &PDFReportService{
		reporter: NewReporter(),
	}
}

func (s *PDFReportService) CreateReport(ctx context.Context, data *models.ReportData) (io.ReadCloser, error) {
	pdfBytes, err := s.reporter.GeneratePDF(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("failed to generate report: %w", err)
	}

	return io.NopCloser(bytes.NewReader(pdfBytes)), nil
}
