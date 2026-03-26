package service

import (
	"context"
	"hr-sorter/internal/repository"
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

func calculatePercent(subset, total int) float64 {
	if total == 0 {
		return 0
	}
	return (float64(subset) / float64(total)) * 100
}
