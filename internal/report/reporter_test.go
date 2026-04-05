package report

import (
	"context"
	"hr-sorter/internal/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReporter_GeneratePDF(t *testing.T) {
	// Setup
	reporter := NewReporter()
	ctx := context.Background()

	data := &models.ReportData{
		AccountName: "Test Account",
		ReportDate:  time.Now(),
		KPI: models.ReportKPI{
			TotalSequences: 10,
			ResponseRate:   50.0,
			HireRate:       10.0,
		},
		Funnel: models.Funnel{
			Initial:   10,
			Screening: 5,
			Tech:      2,
			Offer:     1,
			Accepted:  1,
		},
		Sequences: []models.Sequence{
			{
				CompanyName: "Company A",
				VacancyName: "Developer",
				Category:    ptr("Developer"),
				Status:      "accepted",
			},
			{
				CompanyName: "Company B",
				VacancyName: "Lead Developer",
				Category:    ptr("Lead"),
				Status:      "rejected",
			},
		},
	}

	// Execute
	pdf, err := reporter.GeneratePDF(ctx, data)

	// Assert
	assert.NoError(t, err)
	assert.NotEmpty(t, pdf)
}

func ptr(s string) *string {
	return &s
}
