package dto

import "time"

type HHAuthState string

const (
	AuthStateNone         HHAuthState = "none"
	AuthStateWaitIdentify HHAuthState = "wait_identify"
	AuthStateWaitOTP      HHAuthState = "wait_otp"
	AuthStateWaitCaptcha  HHAuthState = "wait_captcha"
	AuthStateWaitRedirect HHAuthState = "wait_redirect"
	AuthStateCompleted    HHAuthState = "completed"
	AuthStateFailed       HHAuthState = "failed"
)

type HHAuthStatus struct {
	State        HHAuthState `json:"state"`
	IdentifyType string      `json:"identify_type,omitempty"` // "phone" or "email"
	CaptchaImg   []byte      `json:"-"`
	ErrorMessage string      `json:"error_message,omitempty"`
}

type HHNegotiation struct {
	ID        string    `json:"id"`
	Vacancy   string    `json:"vacancy"`
	Category  string    `json:"category"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
}

type HHKPI struct {
	ResponseRate float64 `json:"response_rate"`
	HireRate     float64 `json:"hire_rate"`
	TotalApplied int     `json:"total_applied"`
}
