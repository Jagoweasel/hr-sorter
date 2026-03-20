package models

import "time"

type Account struct {
	ID          int64     `db:"id" json:"id"`
	PhoneNumber string    `db:"phone_number" json:"phone_number"`
	Status      string    `db:"status" json:"status"` // "active", "pending_auth"
	SessionPath string    `db:"session_path" json:"session_path"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

type Contact struct {
	ID        int64     `db:"id" json:"id"`
	TGUserID  int64     `db:"tg_user_id" json:"tg_user_id"`
	FirstName string    `db:"first_name" json:"first_name"`
	LastName  string    `db:"last_name" json:"last_name"`
	Username  string    `db:"username" json:"username"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Message struct {
	ID         int64     `db:"id" json:"id"`
	AccountID  int64     `db:"account_id" json:"account_id"`
	ContactID  int64     `db:"contact_id" json:"contact_id"`
	Text       string    `db:"text" json:"text"`
	IsIncoming bool      `db:"is_incoming" json:"is_incoming"`
	Timestamp  time.Time `db:"timestamp" json:"timestamp"`
}
