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
	detailed, err := s.seqRepo.GetAllFullDetails(ctx, accountID)
	if err != nil {
		return nil, err
	}
	data := s.GetReportDataFromSequences(detailed)
	pStats, _ := s.seqRepo.GetPlatformStats(ctx, accountID)
	data.PlatformStats = pStats
	return data, nil
}

func (s *ReportService) GetReportDataFromSequences(detailed []repository.SequenceWithDetails) *ReportData {
	statusMap := make(map[string]int)
	total := len(detailed)

	for _, sd := range detailed {
		statusMap[sd.Status]++
	}

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

	vacancyMap := make(map[string]map[string]int)
	companyMap := make(map[string]map[string]int)
	for _, sd := range detailed {
		if _, ok := vacancyMap[sd.VacancyName]; !ok {
			vacancyMap[sd.VacancyName] = make(map[string]int)
		}
		vacancyMap[sd.VacancyName][sd.Status]++

		if _, ok := companyMap[sd.CompanyName]; !ok {
			companyMap[sd.CompanyName] = make(map[string]int)
		}
		companyMap[sd.CompanyName][sd.Status]++
	}

	return &ReportData{
		TotalResponses: total,
		Funnel:         funnel,
		VacancyStats:   vacancyMap,
		CompanyStats:   companyMap,
		ConversionRate: calculatePercent(offerPlus, screeningPlus),
		AcceptanceRate: calculatePercent(accepted, offerPlus),
	}
}

func (s *ReportService) ExportSummarizedXLSX(ctx context.Context, accountID string) ([]byte, error) {
	detailed, err := s.seqRepo.GetAllFullDetails(ctx, accountID)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer f.Close()

	// Styles
	styleHeader, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#F3F4F6"}, Pattern: 1},
	})
	styleTableHead, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#E5E7EB"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1},
		},
	})
	styleBold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	styleDone, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#D1FAE5"}, Pattern: 1}})

	// 1. Global Sheets
	accountName := "All Accounts"
	if accountID != "" {
		acc, _ := s.accRepo.GetByID(ctx, accountID)
		if acc != nil {
			accountName = acc.Name
		}
	}

	s.addReportSheets(f, "", accountName, detailed, styleHeader, styleTableHead, styleBold, styleDone)

	// 2. Per-Applicant Sheets if "All" is selected
	if accountID == "" {
		byAccount := make(map[string][]repository.SequenceWithDetails)
		for _, sd := range detailed {
			byAccount[sd.AccountName] = append(byAccount[sd.AccountName], sd)
		}

		for name, accSeqs := range byAccount {
			s.addReportSheets(f, name, name, accSeqs, styleHeader, styleTableHead, styleBold, styleDone)
		}
	}

	f.DeleteSheet("Sheet1")
	f.SetActiveSheet(0)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *ReportService) addReportSheets(f *excelize.File, suffix, title string, data []repository.SequenceWithDetails, styleHeader, styleTableHead, styleBold, styleDone int) {
	prefix := ""
	if suffix != "" {
		prefix = suffix + " - "
	}

	overviewName := truncateSheetName(prefix + "Overview")
	detailedName := truncateSheetName(prefix + "Detailed")
	timelineName := truncateSheetName(prefix + "Timeline")

	reportData := s.GetReportDataFromSequences(data)

	// Overview
	f.NewSheet(overviewName)
	f.SetCellValue(overviewName, "A1", "Recruitment Report: "+title)
	f.MergeCell(overviewName, "A1", "B1")
	f.SetCellStyle(overviewName, "A1", "B1", styleHeader)
	f.SetCellValue(overviewName, "A3", "Metric")
	f.SetCellValue(overviewName, "B3", "Value")
	f.SetCellStyle(overviewName, "A3", "B3", styleTableHead)
	f.SetCellValue(overviewName, "A4", "Total Responses")
	f.SetCellValue(overviewName, "B4", reportData.TotalResponses)
	f.SetCellValue(overviewName, "A5", "Interview-to-Offer Conversion")
	f.SetCellValue(overviewName, "B5", fmt.Sprintf("%.1f%%", reportData.ConversionRate))
	f.SetCellValue(overviewName, "A6", "Hire Rate (Total)")
	f.SetCellValue(overviewName, "B6", fmt.Sprintf("%.1f%%", reportData.AcceptanceRate))

	f.SetCellValue(overviewName, "A8", "Recruitment Funnel")
	f.SetCellStyle(overviewName, "A8", "A8", styleTableHead)
	f.SetCellValue(overviewName, "A9", "Stage")
	f.SetCellValue(overviewName, "B9", "Count")
	f.SetCellStyle(overviewName, "A9", "B9", styleTableHead)
	for i, step := range reportData.Funnel {
		row := i + 10
		f.SetCellValue(overviewName, fmt.Sprintf("A%d", row), step.Label)
		f.SetCellValue(overviewName, fmt.Sprintf("B%d", row), step.Count)
	}

	// Detailed
	f.NewSheet(detailedName)
	headers := []string{"Recruiter TG", "Company", "Date", "Applicant (Account)", "Vacancy Link", "Last Stage", "Status", "Reason", "Comment"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(detailedName, cell, h)
	}
	f.SetCellStyle(detailedName, "A1", "I1", styleTableHead)
	for i, sd := range data {
		row := i + 2
		tgName := ""
		if len(sd.Recruiters) > 0 {
			tgName = deref(sd.Recruiters[0].Username)
		}
		lastStage := "Initial"
		if len(sd.History) > 0 {
			lastStage = sd.History[len(sd.History)-1].Name
		}
		status := "waiting"
		if sd.Sequence.Status == "accepted" {
			status = "accept"
		} else if sd.Sequence.Status == "rejected" {
			status = "decline"
		}

		f.SetCellValue(detailedName, fmt.Sprintf("A%d", row), tgName)
		f.SetCellValue(detailedName, fmt.Sprintf("B%d", row), sd.CompanyName)
		f.SetCellValue(detailedName, fmt.Sprintf("C%d", row), sd.CreatedAt.Format("2006-01-02"))
		f.SetCellValue(detailedName, fmt.Sprintf("D%d", row), sd.AccountName)
		f.SetCellValue(detailedName, fmt.Sprintf("E%d", row), deref(sd.VacancyLink))
		f.SetCellValue(detailedName, fmt.Sprintf("F%d", row), lastStage)
		f.SetCellValue(detailedName, fmt.Sprintf("G%d", row), status)
		f.SetCellValue(detailedName, fmt.Sprintf("H%d", row), deref(sd.RejectionReason))
		f.SetCellValue(detailedName, fmt.Sprintf("I%d", row), deref(sd.SummaryComment))
	}

	// Timeline
	f.NewSheet(timelineName)
	for i, sd := range data {
		row := i + 1
		f.SetCellValue(timelineName, fmt.Sprintf("A%d", row), sd.AccountName)
		f.SetCellValue(timelineName, fmt.Sprintf("B%d", row), sd.CompanyName)
		f.SetCellStyle(timelineName, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), styleBold)

		// Skip cell C (col 3)
		colIdx := 4
		for _, st := range sd.History {
			cell, _ := excelize.CoordinatesToCellName(colIdx, row)
			f.SetCellValue(timelineName, cell, st.Name)
			f.SetCellStyle(timelineName, cell, cell, styleDone)
			colIdx++
		}
	}
}

func truncateSheetName(name string) string {
	if len(name) > 31 {
		return name[:31]
	}
	return name
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
