package service

import (
	"bytes"
	"context"
	"fmt"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/repository"
	"strings"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
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

type PDFExportOptions struct {
	IncludeKPIs      bool
	IncludeFunnel    bool
	IncludeVacancies bool
	IncludeCompanies bool
	IncludeDetailed  bool
	IncludeTimeline  bool
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
		AcceptanceRate: calculatePercent(accepted, total),
	}
}

type reportStyles struct {
	Header         int
	TableHead      int
	Bold           int
	Done           int
	StatusAccept   int
	StatusDecline  int
	StatusWaiting  int
	StageInitial   int
	StageScreening int
	StageTech      int
	StageFinal     int
	StageOffer     int
}

func (s *ReportService) createStyles(f *excelize.File) reportStyles {
	h, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#F3F4F6"}, Pattern: 1},
	})
	th, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#E5E7EB"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1},
		},
	})
	bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})

	done, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#D1FAE5"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1},
		},
	})

	sAccept, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#BBF7D0"}, Pattern: 1}, Font: &excelize.Font{Bold: true}, Border: []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}})
	sDecline, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#FECACA"}, Pattern: 1}, Font: &excelize.Font{Bold: true}, Border: []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}})
	sWaiting, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#FEF08A"}, Pattern: 1}, Font: &excelize.Font{Bold: true}, Border: []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}})

	stInitial, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#DBEAFE"}, Pattern: 1}, Border: []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}})
	stScreening, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#E0E7FF"}, Pattern: 1}, Border: []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}})
	stTech, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#F3E8FF"}, Pattern: 1}, Border: []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}})
	stFinal, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#FCE7F3"}, Pattern: 1}, Border: []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}})
	stOffer, _ := f.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Color: []string{"#FEF9C3"}, Pattern: 1}, Border: []excelize.Border{{Type: "left", Color: "000000", Style: 1}, {Type: "top", Color: "000000", Style: 1}, {Type: "bottom", Color: "000000", Style: 1}, {Type: "right", Color: "000000", Style: 1}}})

	return reportStyles{
		Header:         h,
		TableHead:      th,
		Bold:           bold,
		Done:           done,
		StatusAccept:   sAccept,
		StatusDecline:  sDecline,
		StatusWaiting:  sWaiting,
		StageInitial:   stInitial,
		StageScreening: stScreening,
		StageTech:      stTech,
		StageFinal:     stFinal,
		StageOffer:     stOffer,
	}
}

func (s *ReportService) ExportSummarizedXLSX(ctx context.Context, accountID string) ([]byte, error) {
	detailed, err := s.seqRepo.GetAllFullDetails(ctx, accountID)
	if err != nil {
		return nil, err
	}
	logger.Debug(logger.Reports, "Export requested for AccountID: %s. Fetched %d sequences.", accountID, len(detailed))

	f := excelize.NewFile()
	defer f.Close()
	styles := s.createStyles(f)

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

	// 1. Write Overall Summary to Overview
	currentRow = s.writeOverviewSection(f, sheetOverview, "Recruitment Report: "+accountName, detailed, currentRow, styles)

	// Group by Account ID
	type accGroup struct {
		ID   int64
		Name string
		Slug string
		Seqs []repository.SequenceWithDetails
	}
	byAccountID := make(map[int64]*accGroup)
	var accountIDs []int64

	for _, sd := range detailed {
		id := int64(0)
		if sd.AccountID != nil {
			id = *sd.AccountID
		}

		if _, ok := byAccountID[id]; !ok {
			byAccountID[id] = &accGroup{ID: id, Name: sd.AccountName, Slug: sd.AccountSlug, Seqs: nil}
			accountIDs = append(accountIDs, id)
		}
		byAccountID[id].Seqs = append(byAccountID[id].Seqs, sd)
	}
	logger.Debug(logger.Reports, "Found %d unique applicants in dataset.", len(accountIDs))

	// 2. Add individual summaries to Overview vertically
	if accountID == "" && len(accountIDs) > 0 {
		currentRow += 2
		for _, id := range accountIDs {
			group := byAccountID[id]
			logger.Debug(logger.Reports, "Adding summary block for applicant: %s (ID: %d) to Overview", group.Name, id)
			currentRow = s.writeOverviewSection(f, sheetOverview, "Applicant Summary: "+group.Name, group.Seqs, currentRow, styles)
			currentRow += 2
		}
	}

	// 3. Global Detailed and Timeline
	sheetDetailed := "Detailed Applicants"
	f.NewSheet(sheetDetailed)
	logger.Debug(logger.Reports, "Writing global Detailed sheet")
	s.writeDetailedTable(f, sheetDetailed, detailed, styles)

	sheetTimeline := "Timeline"
	f.NewSheet(sheetTimeline)
	logger.Debug(logger.Reports, "Writing global Timeline sheet")
	s.writeTimelineRows(f, sheetTimeline, detailed, styles)

	// 4. Per-Applicant Sheets
	if accountID == "" {
		for _, id := range accountIDs {
			group := byAccountID[id]
			slug := group.Slug
			if slug == "" {
				slug = fmt.Sprintf("acc_%d", id)
			}

			detName := generateSafeSheetName(id, slug, "Detailed")
			timeName := generateSafeSheetName(id, slug, "Timeline")

			logger.Debug(logger.Reports, "Creating individual sheet: '%s'", detName)
			f.NewSheet(detName)
			s.writeDetailedTable(f, detName, group.Seqs, styles)

			logger.Debug(logger.Reports, "Creating individual sheet: '%s'", timeName)
			f.NewSheet(timeName)
			s.writeTimelineRows(f, timeName, group.Seqs, styles)
		}
	}

	f.SetActiveSheet(0)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		logger.Debug(logger.Reports, "Error writing XLSX to buffer: %v", err)
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *ReportService) writeOverviewSection(f *excelize.File, sheet, title string, data []repository.SequenceWithDetails, startRow int, styles reportStyles) int {
	rd := s.GetReportDataFromSequences(data)
	row := startRow
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), title)
	f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row))
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), styles.Header)

	row += 2
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Metric")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Value")
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), styles.TableHead)

	row++
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Total Responses")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), rd.TotalResponses)
	row++
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Interview-to-Offer Conversion")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), fmt.Sprintf("%.1f%%", rd.ConversionRate))
	row++
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Hire Rate (Total)")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), fmt.Sprintf("%.1f%%", rd.AcceptanceRate))

	row += 2
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Recruitment Funnel")
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), styles.TableHead)
	row++
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Stage")
	f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Count")
	f.SetCellValue(sheet, fmt.Sprintf("C%d", row), "Percentage")
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), styles.TableHead)

	for _, step := range rd.Funnel {
		row++
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), step.Label)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), step.Count)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), fmt.Sprintf("%.0f%%", step.Percentage))
	}
	return row
}

func (s *ReportService) writeDetailedTable(f *excelize.File, sheet string, data []repository.SequenceWithDetails, styles reportStyles) {
	headers := []string{"Recruiter TG", "Company", "Date", "Applicant (Account)", "Vacancy Link", "Last Stage", "Status", "Reason", "Comment"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	f.SetCellStyle(sheet, "A1", "I1", styles.TableHead)

	for i, sd := range data {
		row := i + 2
		tgName := "HH"
		if len(sd.Recruiters) > 0 {
			r := sd.Recruiters[0]
			if r.Platform == "tg" {
				if r.Username != nil && *r.Username != "" {
					tgName = "@" + *r.Username
				} else {
					tgName = deref(r.FirstName) + " " + deref(r.LastName)
				}
			}
		}

		lastStage := "Initial"
		lastStageStyle := styles.StageInitial
		if len(sd.History) > 0 {
			lastStage = sd.History[len(sd.History)-1].Name
		}

		lsLower := strings.ToLower(lastStage)
		switch {
		case strings.Contains(lsLower, "screening"):
			lastStageStyle = styles.StageScreening
		case strings.Contains(lsLower, "tech"):
			lastStageStyle = styles.StageTech
		case strings.Contains(lsLower, "final"):
			lastStageStyle = styles.StageFinal
		case strings.Contains(lsLower, "offer"):
			lastStageStyle = styles.StageOffer
		}

		status := "waiting"
		statusStyle := styles.StatusWaiting
		if sd.Sequence.Status == "accepted" {
			status = "accept"
			statusStyle = styles.StatusAccept
		}
		if sd.Sequence.Status == "rejected" {
			status = "decline"
			statusStyle = styles.StatusDecline
		}

		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), tgName)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), sd.CompanyName)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), sd.CreatedAt.Format("2006-01-02"))
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), sd.AccountName)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), deref(sd.VacancyLink))

		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), lastStage)
		f.SetCellStyle(sheet, fmt.Sprintf("F%d", row), fmt.Sprintf("F%d", row), lastStageStyle)

		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), status)
		f.SetCellStyle(sheet, fmt.Sprintf("G%d", row), fmt.Sprintf("G%d", row), statusStyle)

		f.SetCellValue(sheet, fmt.Sprintf("H%d", row), deref(sd.RejectionReason))
		f.SetCellValue(sheet, fmt.Sprintf("I%d", row), deref(sd.SummaryComment))
	}
	f.SetColWidth(sheet, "A", "I", 25)
}

func (s *ReportService) writeTimelineRows(f *excelize.File, sheet string, data []repository.SequenceWithDetails, styles reportStyles) {
	for i, sd := range data {
		row := i + 1
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), sd.AccountName)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), sd.CompanyName)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), styles.Bold)

		// Skip Col C (spacer)
		colIdx := 4
		for _, st := range sd.History {
			cell, _ := excelize.CoordinatesToCellName(colIdx, row)
			f.SetCellValue(sheet, cell, st.Name)
			f.SetCellStyle(sheet, cell, cell, styles.Done)
			colIdx++
		}
	}
	f.SetColWidth(sheet, "A", "B", 25)
	f.SetColWidth(sheet, "C", "C", 5)
	f.SetColWidth(sheet, "D", "Z", 20)
}

func generateSafeSheetName(id int64, slug, suffix string) string {
	if slug == "" {
		slug = fmt.Sprintf("acc_%d", id)
	}
	name := slug + "_" + strings.ToLower(suffix)
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

func (s *ReportService) ExportPDF(ctx context.Context, accountID string, opts PDFExportOptions) ([]byte, error) {
	detailed, err := s.seqRepo.GetAllFullDetails(ctx, accountID)
	if err != nil {
		return nil, err
	}

	accountName := "All Accounts"
	if accountID != "" {
		acc, _ := s.accRepo.GetByID(ctx, accountID)
		if acc != nil {
			accountName = acc.Name
		}
	}

	m := pdf.NewMaroto(consts.Landscape, consts.A4)
	m.SetPageMargins(10, 10, 10)

	// Header
	m.RegisterHeader(func() {
		m.Row(10, func() {
			m.Col(12, func() {
				m.Text("Recruitment Report: "+accountName, props.Text{
					Top:   2,
					Size:  16,
					Style: consts.Bold,
					Align: consts.Center,
				})
			})
		})
	})

	// 1. Overall Summary
	if opts.IncludeKPIs || opts.IncludeFunnel {
		rd := s.GetReportDataFromSequences(detailed)
		s.writePDFOverviewSection(m, "Overall Summary", rd, opts)
	}

	// Group by Account
	byAccount := make(map[int64][]repository.SequenceWithDetails)
	var accountOrder []int64
	accNames := make(map[int64]string)
	for _, sd := range detailed {
		id := int64(0)
		if sd.AccountID != nil {
			id = *sd.AccountID
		}
		if _, ok := byAccount[id]; !ok {
			accountOrder = append(accountOrder, id)
			accNames[id] = sd.AccountName
		}
		byAccount[id] = append(byAccount[id], sd)
	}

	// 2. Per-Applicant Summaries
	if accountID == "" && len(accountOrder) > 1 {
		for _, id := range accountOrder {
			accSeqs := byAccount[id]
			rd := s.GetReportDataFromSequences(accSeqs)
			m.AddPage()
			s.writePDFOverviewSection(m, "Applicant Summary: "+accNames[id], rd, opts)
		}
	}

	// 3. Vacancy Stats
	if opts.IncludeVacancies {
		m.AddPage()
		rd := s.GetReportDataFromSequences(detailed)
		m.Row(10, func() {
			m.Col(12, func() { m.Text("Performance by Vacancy", props.Text{Size: 14, Style: consts.Bold}) })
		})
		headers := []string{"Vacancy", "Total", "Screening+", "Offer+", "Accepted"}
		var contents [][]string
		for name, stats := range rd.VacancyStats {
			total := 0
			for _, c := range stats {
				total += c
			}
			screeningPlus := stats["screening"] + stats["tech"] + stats["final"] + stats["offer"] + stats["accepted"]
			offerPlus := stats["offer"] + stats["accepted"]
			accepted := stats["accepted"]
			contents = append(contents, []string{name, fmt.Sprintf("%d", total), fmt.Sprintf("%d", screeningPlus), fmt.Sprintf("%d", offerPlus), fmt.Sprintf("%d", accepted)})
		}
		m.TableList(headers, contents, props.TableList{
			HeaderProp:  props.TableListContent{Size: 9, Style: consts.Bold},
			ContentProp: props.TableListContent{Size: 8},
		})
	}

	// 4. Company Stats
	if opts.IncludeCompanies {
		m.AddPage()
		rd := s.GetReportDataFromSequences(detailed)
		m.Row(10, func() {
			m.Col(12, func() { m.Text("Performance by Company", props.Text{Size: 14, Style: consts.Bold}) })
		})
		headers := []string{"Company", "Total", "Offers", "Accepted"}
		var contents [][]string
		for name, stats := range rd.CompanyStats {
			total := 0
			for _, c := range stats {
				total += c
			}
			offerPlus := stats["offer"] + stats["accepted"]
			accepted := stats["accepted"]
			contents = append(contents, []string{name, fmt.Sprintf("%d", total), fmt.Sprintf("%d", offerPlus), fmt.Sprintf("%d", accepted)})
		}
		m.TableList(headers, contents, props.TableList{
			HeaderProp:  props.TableListContent{Size: 9, Style: consts.Bold},
			ContentProp: props.TableListContent{Size: 8},
		})
	}

	// 5. Detailed Applicants
	if opts.IncludeDetailed {
		m.AddPage()
		m.Row(10, func() {
			m.Col(12, func() { m.Text("Detailed Applicants", props.Text{Size: 14, Style: consts.Bold}) })
		})
		headers := []string{"Recruiter", "Company", "Date", "Applicant", "Last Stage", "Status"}
		var contents [][]string
		for _, sd := range detailed {
			tg := "HH"
			if len(sd.Recruiters) > 0 {
				r := sd.Recruiters[0]
				if r.Platform == "tg" {
					if r.Username != nil && *r.Username != "" {
						tg = "@" + *r.Username
					} else {
						tg = deref(r.FirstName)
					}
				}
			}
			lastStage := "Initial"
			if len(sd.History) > 0 {
				lastStage = sd.History[len(sd.History)-1].Name
			}

			contents = append(contents, []string{
				tg, sd.CompanyName, sd.CreatedAt.Format("2006-01-02"), sd.AccountName, lastStage, sd.Status,
			})
		}
		m.TableList(headers, contents, props.TableList{
			HeaderProp:  props.TableListContent{Size: 9, Style: consts.Bold},
			ContentProp: props.TableListContent{Size: 8},
		})
	}

	// 6. Timelines
	if opts.IncludeTimeline {
		m.AddPage()
		m.Row(10, func() {
			m.Col(12, func() { m.Text("Applicant Timelines", props.Text{Size: 14, Style: consts.Bold}) })
		})
		for _, sd := range detailed {
			m.Row(10, func() {
				m.Col(12, func() {
					m.Text(sd.AccountName+" | "+sd.CompanyName, props.Text{Size: 10, Style: consts.Bold})
				})
			})
			m.Row(8, func() {
				for _, st := range sd.History {
					m.Col(2, func() {
						m.Text(st.Name, props.Text{
							Size: 7, Align: consts.Center, Top: 1,
							Color: color.Color{Red: 16, Green: 185, Blue: 129}, // Green
						})
					})
				}
			})
			m.Line(5)
		}
	}

	buf, err := m.Output()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *ReportService) writePDFOverviewSection(m pdf.Maroto, title string, rd *ReportData, opts PDFExportOptions) {
	m.Row(10, func() {
		m.Col(12, func() { m.Text(title, props.Text{Size: 14, Style: consts.Bold}) })
	})

	if opts.IncludeKPIs {
		m.Row(8, func() {
			m.Col(12, func() { m.Text("Key Performance Indicators", props.Text{Size: 10, Style: consts.Bold}) })
		})
		m.Row(6, func() {
			m.Col(4, func() { m.Text("Total Responses:", props.Text{Size: 9}) })
			m.Col(2, func() { m.Text(fmt.Sprintf("%d", rd.TotalResponses), props.Text{Size: 9, Style: consts.Bold}) })
		})
		m.Row(6, func() {
			m.Col(4, func() { m.Text("Conv. Rate (Intv/Offer):", props.Text{Size: 9}) })
			m.Col(2, func() { m.Text(fmt.Sprintf("%.1f%%", rd.ConversionRate), props.Text{Size: 9, Style: consts.Bold}) })
		})
		m.Row(6, func() {
			m.Col(4, func() { m.Text("Hire Rate (Total):", props.Text{Size: 9}) })
			m.Col(2, func() { m.Text(fmt.Sprintf("%.1f%%", rd.AcceptanceRate), props.Text{Size: 9, Style: consts.Bold}) })
		})
	}

	if opts.IncludeFunnel {
		m.Row(10, func() {
			m.Col(12, func() { m.Text("Recruitment Funnel", props.Text{Size: 10, Style: consts.Bold, Top: 4}) })
		})
		headers := []string{"Stage", "Count", "Percentage"}
		var contents [][]string
		for _, step := range rd.Funnel {
			contents = append(contents, []string{step.Label, fmt.Sprintf("%d", step.Count), fmt.Sprintf("%.0f%%", step.Percentage)})
		}
		m.TableList(headers, contents, props.TableList{
			HeaderProp:  props.TableListContent{Size: 9, Style: consts.Bold},
			ContentProp: props.TableListContent{Size: 8},
		})
	}
}
