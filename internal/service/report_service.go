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
	accRepo *repository.AccountRepository
}

func NewReportService(seqRepo *repository.SequenceRepository, accRepo *repository.AccountRepository) *ReportService {
	return &ReportService{
		seqRepo: seqRepo,
		accRepo: accRepo,
	}
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

	detailed, err := s.seqRepo.GetAllFullDetails(ctx, accountID)
	if err != nil {
		return nil, err
	}

	accountName := "All Accounts"
	if accountID != "" {
		acc, err := s.accRepo.GetByID(ctx, accountID)
		if err == nil {
			accountName = acc.Name
		}
	}

	f := excelize.NewFile()
	defer f.Close()

	styleHeader, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#F3F4F6"}, Pattern: 1},
	})
	styleTableHead, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#E5E7EB"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
		},
	})

	// 1. Overview Sheet
	sheet := "Overview"
	f.SetSheetName("Sheet1", sheet)
	f.SetCellValue(sheet, "A1", "Recruitment Report: "+accountName)
	f.MergeCell(sheet, "A1", "B1")
	f.SetCellStyle(sheet, "A1", "B1", styleHeader)

	f.SetCellValue(sheet, "A3", "Metric")
	f.SetCellValue(sheet, "B3", "Value")
	f.SetCellStyle(sheet, "A3", "B3", styleTableHead)

	f.SetCellValue(sheet, "A4", "Total Responses")
	f.SetCellValue(sheet, "B4", data.TotalResponses)
	f.SetCellValue(sheet, "A5", "Interview-to-Offer Conversion")
	f.SetCellValue(sheet, "B5", fmt.Sprintf("%.1f%%", data.ConversionRate))
	f.SetCellValue(sheet, "A6", "Hire Rate (Total)")
	f.SetCellValue(sheet, "B6", fmt.Sprintf("%.1f%%", data.AcceptanceRate))

	// Funnel in Overview
	f.SetCellValue(sheet, "A8", "Recruitment Funnel")
	f.SetCellStyle(sheet, "A8", "A8", styleTableHead)
	f.SetCellValue(sheet, "A9", "Stage")
	f.SetCellValue(sheet, "B9", "Count")
	f.SetCellStyle(sheet, "A9", "B9", styleTableHead)
	for i, step := range data.Funnel {
		row := i + 10
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), step.Label)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), step.Count)
	}

	// 2. Detailed Applicants
	sheet = "Detailed Applicants"
	f.NewSheet(sheet)
	headers := []string{"TG Name", "Company", "Date", "Applicant Name", "Vacancy Link", "Last Stage", "Status", "Reason", "Comment"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	f.SetCellStyle(sheet, "A1", "I1", styleTableHead)

	for i, s := range detailed {
		row := i + 2
		tgName := ""
		appName := ""
		if len(s.Recruiters) > 0 {
			r := s.Recruiters[0]
			tgName = deref(r.Username)
			firstName := deref(r.FirstName)
			lastName := deref(r.LastName)
			if firstName != "" || lastName != "" {
				appName = fmt.Sprintf("%s %s", firstName, lastName)
			}
		}

		lastStage := "Initial"
		if len(s.History) > 0 {
			lastStage = s.History[len(s.History)-1].Name
		}

		status := "waiting"
		if s.Sequence.Status == "accepted" {
			status = "accept"
		} else if s.Sequence.Status == "rejected" {
			status = "decline"
		}

		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), tgName)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), s.CompanyName)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), s.CreatedAt.Format("2006-01-02"))
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), appName)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), deref(s.VacancyLink))
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), lastStage)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), status)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", row), deref(s.RejectionReason))
		f.SetCellValue(sheet, fmt.Sprintf("I%d", row), deref(s.SummaryComment))
	}

	// 3. Timelines
	sheet = "Timelines"
	f.NewSheet(sheet)
	for i, s := range detailed {
		row := i + 1
		appName := s.CompanyName
		if len(s.Recruiters) > 0 {
			appName += " (" + deref(s.Recruiters[0].FirstName) + ")"
		}
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), appName)

		colIdx := 2
		for _, st := range s.Stages {
			cell, _ := excelize.CoordinatesToCellName(colIdx, row)
			val := st.Name
			if st.IsCompleted {
				val += " (Done)"
				style, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#D1FAE5"}, Pattern: 1}})
				f.SetCellStyle(sheet, cell, cell, style)
			}
			f.SetCellValue(sheet, cell, val)
			colIdx++
		}
	}

	f.SetActiveSheet(0)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func calculatePercent(subset, total int) float64 {
	if total == 0 {
		return 0
	}
	return (float64(subset) / float64(total)) * 100
}
