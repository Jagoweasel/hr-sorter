package service

import (
	"context"
	"fmt"
	"hr-sorter/internal/domain"
	"hr-sorter/internal/models"
	"hr-sorter/internal/report"
	"hr-sorter/internal/repository"
	"regexp"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type ReportService struct {
	seqRepo  *repository.SequenceRepository
	accRepo  *repository.AccountRepository
	mapRepo  *repository.MappingRepository
	reporter domain.Reporter
}

func NewReportService(seqRepo *repository.SequenceRepository, accRepo *repository.AccountRepository, mapRepo *repository.MappingRepository) *ReportService {
	return &ReportService{
		seqRepo:  seqRepo,
		accRepo:  accRepo,
		mapRepo:  mapRepo,
		reporter: report.NewReporter(),
	}
}

type FunnelStep struct {
	Label      string
	Count      int
	Percentage float64
}

type ReportData struct {
	TotalApplications int
	TotalResponses    int
	Funnel            []FunnelStep
	VacancyStats      map[string]map[string]int
	CompanyStats      map[string]map[string]int
	PlatformStats     []repository.PlatformStats
	ConversionRate    float64
	AcceptanceRate    float64
	AccountName       string
	Detailed          []repository.SequenceWithDetails
}

type PDFExportOptions struct {
	IncludeKPIs      bool
	IncludeFunnel    bool
	IncludeVacancies bool
	IncludeCompanies bool
	IncludeDetailed  bool
	IncludeTimeline  bool
}

type mappedRule struct {
	re       *regexp.Regexp
	category string
}

func (s *ReportService) GetReportData(ctx context.Context, accountID string) (*ReportData, error) {
	detailed, err := s.seqRepo.GetAllFullDetails(ctx, accountID)
	if err != nil {
		return nil, err
	}

	mappingRules, _ := s.mapRepo.GetAll(ctx)
	var rules []mappedRule
	for _, r := range mappingRules {
		re, err := regexp.Compile("(?i)" + r.Pattern)
		if err == nil {
			rules = append(rules, mappedRule{re: re, category: r.Category})
		}
	}

	data := s.GetReportDataFromSequences(detailed, rules)
	pStats, _ := s.seqRepo.GetPlatformStats(ctx, accountID)
	data.PlatformStats = pStats
	data.Detailed = detailed

	totalApps, _ := s.seqRepo.GetTotalApplications(ctx, accountID)
	data.TotalApplications = totalApps

	if accountID != "" {
		acc, _ := s.accRepo.GetByID(ctx, accountID)
		if acc != nil {
			data.AccountName = acc.Name
		}
	} else {
		data.AccountName = "All Accounts"
	}

	if totalApps > 0 {
		newFunnel := []FunnelStep{
			{Label: "Total Applications", Count: totalApps, Percentage: 100},
		}
		for i, step := range data.Funnel {
			step.Percentage = calculatePercent(step.Count, totalApps)
			if i == 0 {
				step.Label = "Responses"
			}
			newFunnel = append(newFunnel, step)
		}
		data.Funnel = newFunnel
	}

	return data, nil
}

func (s *ReportService) GetReportDataFromSequences(detailed []repository.SequenceWithDetails, rules []mappedRule) *ReportData {
	statusMap := make(map[string]int)
	total := len(detailed)

	for _, sd := range detailed {
		statusMap[sd.Status]++
	}

	screeningPlus := statusMap["screening"] + statusMap["tech"] + statusMap["final"] + statusMap["offer"] + statusMap["accepted"]
	offerPlus := statusMap["offer"] + statusMap["accepted"]
	accepted := statusMap["accepted"]

	funnel := []FunnelStep{
		{Label: "Total Responses", Count: total, Percentage: 100},
		{Label: "Interviews", Count: screeningPlus, Percentage: calculatePercent(screeningPlus, total)},
		{Label: "Technical", Count: statusMap["tech"] + statusMap["final"] + statusMap["offer"] + statusMap["accepted"], Percentage: 0},
		{Label: "Offers", Count: offerPlus, Percentage: calculatePercent(offerPlus, total)},
		{Label: "Hires", Count: accepted, Percentage: calculatePercent(accepted, total)},
	}

	vacancyMap := make(map[string]map[string]int)
	companyMap := make(map[string]map[string]int)
	for _, sd := range detailed {
		name := sd.VacancyName
		for _, r := range rules {
			if r.re.MatchString(name) {
				name = r.category
				break
			}
		}

		if _, ok := vacancyMap[name]; !ok {
			vacancyMap[name] = make(map[string]int)
		}
		vacancyMap[name][sd.Status]++

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

func (s *ReportService) ExportPDF(ctx context.Context, accountID string, opts PDFExportOptions) ([]byte, error) {
	mainData, err := s.GetReportData(ctx, accountID)
	if err != nil {
		return nil, err
	}

	mrd := &models.ReportData{
		AccountName: mainData.AccountName,
		ReportDate:  time.Now(),
		KPI: models.ReportKPI{
			TotalApplied:   mainData.TotalApplications,
			TotalSequences: mainData.TotalResponses,
			ResponseRate:   mainData.ConversionRate,
			HireRate:       mainData.AcceptanceRate,
		},
		Funnel: models.Funnel{
			Applied: mainData.TotalApplications,
			Initial: mainData.TotalResponses,
		},
	}

	for _, step := range mainData.Funnel {
		switch step.Label {
		case "Interviews":
			mrd.Funnel.Screening = step.Count
		case "Technical":
			mrd.Funnel.Tech = step.Count
		case "Offers":
			mrd.Funnel.Offer = step.Count
		case "Hires":
			mrd.Funnel.Accepted = step.Count
		}
	}

	for _, sd := range mainData.Detailed {
		mrd.Sequences = append(mrd.Sequences, sd.Sequence)
	}

	if accountID == "" {
		accounts, _ := s.accRepo.GetAll(ctx)
		for _, acc := range accounts {
			accData, err := s.GetReportData(ctx, fmt.Sprintf("%d", acc.ID))
			if err != nil || accData.TotalResponses == 0 {
				continue
			}

			section := models.ReportSection{
				AccountName: acc.Name,
				KPI: models.ReportKPI{
					TotalApplied:   accData.TotalApplications,
					TotalSequences: accData.TotalResponses,
					ResponseRate:   accData.ConversionRate,
					HireRate:       accData.AcceptanceRate,
				},
				Funnel: models.Funnel{
					Applied: accData.TotalApplications,
					Initial: accData.TotalResponses,
				},
			}
			for _, step := range accData.Funnel {
				switch step.Label {
				case "Interviews":
					section.Funnel.Screening = step.Count
				case "Technical":
					section.Funnel.Tech = step.Count
				case "Offers":
					section.Funnel.Offer = step.Count
				case "Hires":
					section.Funnel.Accepted = step.Count
				}
			}
			for _, sd := range accData.Detailed {
				section.Sequences = append(section.Sequences, sd.Sequence)
			}
			mrd.Sections = append(mrd.Sections, section)
		}
	}

	return s.reporter.GeneratePDF(ctx, mrd)
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

	currentRow = s.writeOverviewSection(f, sheetOverview, "Recruitment Report: "+accountName, detailed, currentRow, styles)

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

	if accountID == "" && len(accountIDs) > 0 {
		currentRow += 2
		for _, id := range accountIDs {
			group := byAccountID[id]
			currentRow = s.writeOverviewSection(f, sheetOverview, "Applicant Summary: "+group.Name, group.Seqs, currentRow, styles)
			currentRow += 2
		}
	}

	sheetDetailed := "Detailed Applicants"
	f.NewSheet(sheetDetailed)
	s.writeDetailedTable(f, sheetDetailed, detailed, styles)

	sheetTimeline := "Timeline"
	f.NewSheet(sheetTimeline)
	s.writeTimelineRows(f, sheetTimeline, detailed, styles)

	f.SetActiveSheet(0)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *ReportService) writeOverviewSection(f *excelize.File, sheet, title string, data []repository.SequenceWithDetails, startRow int, styles reportStyles) int {
	rd := s.GetReportDataFromSequences(data, nil)
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
