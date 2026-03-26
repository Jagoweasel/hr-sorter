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
	styleDone, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#D1FAE5"}, Pattern: 1}, Border: []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}})

	// 1. Overview Sheet
	sheetOverview := "Overview"
	f.SetSheetName("Sheet1", sheetOverview)

	currentRow := 1
	accountName := "All Accounts"
	if accountID != "" {
		acc, _ := s.accRepo.GetByID(ctx, accountID)
		if acc != nil {
			accountName = acc.Name
		}
	}

	// Write Overall Summary to Overview
	currentRow = s.writeOverviewSection(f, sheetOverview, "Recruitment Report: "+accountName, detailed, currentRow, styleHeader, styleTableHead)

	// If multiple accounts, add vertical breakdowns
	byAccount := make(map[string][]repository.SequenceWithDetails)
	for _, sd := range detailed {
		byAccount[sd.AccountName] = append(byAccount[sd.AccountName], sd)
	}

	if accountID == "" && len(byAccount) > 1 {
		currentRow += 2 // Gap
		for name, accSeqs := range byAccount {
			currentRow = s.writeOverviewSection(f, sheetOverview, "Applicant Summary: "+name, accSeqs, currentRow, styleHeader, styleTableHead)
			currentRow += 2 // Gap between applicants
		}
	}

	// 2. Global Detailed and Timeline
	sheetDetailed := "Detailed Applicants"
	f.NewSheet(sheetDetailed)
	s.writeDetailedTable(f, sheetDetailed, detailed, styleTableHead)

	sheetTimeline := "Timeline"
	f.NewSheet(sheetTimeline)
	s.writeTimelineRows(f, sheetTimeline, detailed, styleBold, styleDone)

	// 3. Per-Applicant Sheets if "All" is selected
	if accountID == "" && len(byAccount) > 1 {
		for name, accSeqs := range byAccount {
			detSheet := truncateSheetName(name + " - Detailed")
			timeSheet := truncateSheetName(name + " - Timeline")

			f.NewSheet(detSheet)
			s.writeDetailedTable(f, detSheet, accSeqs, styleTableHead)

			f.NewSheet(timeSheet)
			s.writeTimelineRows(f, timeSheet, accSeqs, styleBold, styleDone)
		}
	}

	f.SetActiveSheet(0)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *ReportService) writeOverviewSection(f *excelize.File, sheet, title string, data []repository.SequenceWithDetails, startRow int, styleHeader, styleTableHead int) int {
	reportData := s.GetReportDataFromSequences(data)

	row := startRow
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), title)
	f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row))
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), styleHeader)

	row += 2
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Metric")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Value")
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), styleTableHead)

	row++
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Total Responses")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), reportData.TotalResponses)
	row++
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Interview-to-Offer Conversion")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), fmt.Sprintf("%.1f%%", reportData.ConversionRate))
	row++
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Hire Rate (Total)")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), fmt.Sprintf("%.1f%%", reportData.AcceptanceRate))

	row += 2
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Recruitment Funnel")
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), styleTableHead)
	row++
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Stage")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Count")
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), styleTableHead)

	for _, step := range reportData.Funnel {
		row++
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), step.Label)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), step.Count)
	}

	return row
}

func (s *ReportService) writeDetailedTable(f *excelize.File, sheet string, data []repository.SequenceWithDetails, styleTableHead int) {
	headers := []string{"Recruiter TG", "Company", "Date", "Applicant (Account)", "Vacancy Link", "Last Stage", "Status", "Reason", "Comment"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	f.SetCellStyle(sheet, "A1", "I1", styleTableHead)

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

		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), tgName)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), sd.CompanyName)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), sd.CreatedAt.Format("2006-01-02"))
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), sd.AccountName)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), deref(sd.VacancyLink))
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), lastStage)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), status)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", row), deref(sd.RejectionReason))
		f.SetCellValue(sheet, fmt.Sprintf("I%d", row), deref(sd.SummaryComment))
	}
	f.SetColWidth(sheet, "A", "I", 20)
}

func (s *ReportService) writeTimelineRows(f *excelize.File, sheet string, data []repository.SequenceWithDetails, styleBold, styleDone int) {
	for i, sd := range data {
		row := i + 1
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), sd.AccountName)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), sd.CompanyName)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), styleBold)

		// Skip cell C (col 3)
		colIdx := 4
		for _, st := range sd.History {
			cell, _ := excelize.CoordinatesToCellName(colIdx, row)
			f.SetCellValue(sheet, cell, st.Name)
			f.SetCellStyle(sheet, cell, cell, styleDone)
			colIdx++
		}
	}
	f.SetColWidth(sheet, "A", "B", 25)
	f.SetColWidth(sheet, "C", "C", 5)
	f.SetColWidth(sheet, "D", "Z", 15)
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
