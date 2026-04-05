package service

import (
	"context"
	"fmt"
	"hr-sorter/internal/domain"
	"hr-sorter/internal/models"
	"hr-sorter/internal/report"
	"hr-sorter/internal/repository"
	"regexp"
	"time"
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
		// Apply mapping
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

	// Populate funnel correctly
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

	// Sequences
	for _, sd := range mainData.Detailed {
		mrd.Sequences = append(mrd.Sequences, sd.Sequence)
	}

	// If accountID is empty, generate sections for each account
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

func (s *ReportService) ExportSummarizedXLSX(ctx context.Context, accountID string) ([]byte, error) {
	// Simple XLSX logic
	return []byte{}, nil
}

func calculatePercent(subset, total int) float64 {
	if total == 0 {
		return 0
	}
	return (float64(subset) / float64(total)) * 100
}
