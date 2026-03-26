package service

import (
	"bytes"
	"context"
	"fmt"
	"hr-sorter/internal/repository"

	"github.com/xuri/excelize/v2"
)

type ReportService struct {
	seqRepo *repository.SequenceRepository
}

func NewReportService(seqRepo *repository.SequenceRepository) *ReportService {
	return &ReportService{seqRepo: seqRepo}
}

type FunnelStep struct {
	Label      string
	Count      int
	Percentage float64 // Percentage of total responses
}

type ReportData struct {
	TotalResponses int
	Funnel         []FunnelStep
	VacancyStats   map[string]map[string]int
	CompanyStats   map[string]map[string]int
	PlatformStats  []repository.PlatformStats
	ConversionRate float64 // Interview to Offer
	AcceptanceRate float64 // Offer to Accepted
}

func (s *ReportService) GetReportData(ctx context.Context, accountID string) (*ReportData, error) {
	counts, err := s.seqRepo.GetStatusCounts(ctx, accountID)
	if err != nil {
		return nil, err
	}

	statusMap := make(map[string]int)
	total := 0
	for _, c := range counts {
		statusMap[c.Status] = c.Count
		total += c.Count
	}

	// Funnel calculation
	// We approximate cumulative counts based on current status
	screeningPlus := statusMap["screening"] + statusMap["tech"] + statusMap["final"] + statusMap["offer"] + statusMap["accepted"]
	techPlus := statusMap["tech"] + statusMap["final"] + statusMap["offer"] + statusMap["accepted"]
	finalPlus := statusMap["final"] + statusMap["offer"] + statusMap["accepted"]
	offerPlus := statusMap["offer"] + statusMap["accepted"]
	accepted := statusMap["accepted"]

	funnel := []FunnelStep{
		{Label: "Total Responses", Count: total, Percentage: 100},
		{Label: "Interviews", Count: screeningPlus, Percentage: calculatePercent(screeningPlus, total)},
		{Label: "Technical", Count: techPlus, Percentage: calculatePercent(techPlus, total)},
		{Label: "Finals", Count: finalPlus, Percentage: calculatePercent(finalPlus, total)},
		{Label: "Offers", Count: offerPlus, Percentage: calculatePercent(offerPlus, total)},
		{Label: "Hires", Count: accepted, Percentage: calculatePercent(accepted, total)},
	}

	vStats, _ := s.seqRepo.GetVacancyStats(ctx, accountID)
	vacancyMap := make(map[string]map[string]int)
	for _, vs := range vStats {
		if _, ok := vacancyMap[vs.VacancyName]; !ok {
			vacancyMap[vs.VacancyName] = make(map[string]int)
		}
		vacancyMap[vs.VacancyName][vs.Status] = vs.Count
	}

	cStats, _ := s.seqRepo.GetCompanyStats(ctx, accountID)
	companyMap := make(map[string]map[string]int)
	for _, cs := range cStats {
		if _, ok := companyMap[cs.CompanyName]; !ok {
			companyMap[cs.CompanyName] = make(map[string]int)
		}
		companyMap[cs.CompanyName][cs.Status] = cs.Count
	}

	pStats, _ := s.seqRepo.GetPlatformStats(ctx, accountID)

	return &ReportData{
		TotalResponses: total,
		Funnel:         funnel,
		VacancyStats:   vacancyMap,
		CompanyStats:   companyMap,
		PlatformStats:  pStats,
		ConversionRate: calculatePercent(offerPlus, screeningPlus),
		AcceptanceRate: calculatePercent(accepted, offerPlus),
	}, nil
}

func (s *ReportService) ExportSummarizedXLSX(ctx context.Context, accountID string) ([]byte, error) {
	data, err := s.GetReportData(ctx, accountID)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer f.Close()

	// 1. Overview Sheet
	sheet := "Overview"
	f.SetSheetName("Sheet1", sheet)
	f.SetCellValue(sheet, "A1", "Metric")
	f.SetCellValue(sheet, "B1", "Value")
	f.SetCellValue(sheet, "A2", "Total Responses")
	f.SetCellValue(sheet, "B2", data.TotalResponses)
	f.SetCellValue(sheet, "A3", "Interview-to-Offer Conversion")
	f.SetCellValue(sheet, "B3", fmt.Sprintf("%.1f%%", data.ConversionRate))
	f.SetCellValue(sheet, "A4", "Hire Rate (Total)")
	f.SetCellValue(sheet, "B4", fmt.Sprintf("%.1f%%", data.AcceptanceRate))

	// Style headers
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#E0E0E0"}, Pattern: 1},
	})
	f.SetCellStyle(sheet, "A1", "B1", style)

	// 2. Funnel Sheet
	sheet = "Recruitment Funnel"
	f.NewSheet(sheet)
	f.SetCellValue(sheet, "A1", "Stage")
	f.SetCellValue(sheet, "B1", "Count")
	f.SetCellValue(sheet, "C1", "Percentage")
	f.SetCellStyle(sheet, "A1", "C1", style)
	for i, step := range data.Funnel {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), step.Label)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), step.Count)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), fmt.Sprintf("%.0f%%", step.Percentage))
	}

	// 3. Vacancy Stats
	sheet = "Vacancies"
	f.NewSheet(sheet)
	headers := []string{"Vacancy", "Initial", "Screening", "Tech", "Final", "Offer", "Accepted", "Rejected", "Total"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	f.SetCellStyle(sheet, "A1", "I1", style)

	rowIdx := 2
	for name, stats := range data.VacancyStats {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIdx), name)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", rowIdx), stats["initial"])
		f.SetCellValue(sheet, fmt.Sprintf("C%d", rowIdx), stats["screening"])
		f.SetCellValue(sheet, fmt.Sprintf("D%d", rowIdx), stats["tech"])
		f.SetCellValue(sheet, fmt.Sprintf("E%d", rowIdx), stats["final"])
		f.SetCellValue(sheet, fmt.Sprintf("F%d", rowIdx), stats["offer"])
		f.SetCellValue(sheet, fmt.Sprintf("G%d", rowIdx), stats["accepted"])
		f.SetCellValue(sheet, fmt.Sprintf("H%d", rowIdx), stats["rejected"])

		total := 0
		for _, c := range stats {
			total += c
		}
		f.SetCellValue(sheet, fmt.Sprintf("I%d", rowIdx), total)
		rowIdx++
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func calculatePercent(subset, total int) float64 {
	if total == 0 {
		return 0
	}
	return (float64(subset) / float64(total)) * 100
}
