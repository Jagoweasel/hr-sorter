package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type StateRepository struct {
	db *sqlx.DB
}

func NewStateRepository(db *sqlx.DB) *StateRepository {
	return &StateRepository{db: db}
}

func (r *StateRepository) GetDB() *sqlx.DB {
	return r.db
}

type TGState struct {
	IntegrationID int64 `db:"integration_id"`
	Pts           int   `db:"pts"`
	Qts           int   `db:"qts"`
	Seq           int   `db:"seq"`
	Date          int   `db:"date"`
}

func (r *StateRepository) GetTGState(ctx context.Context, integrationID int64) (*TGState, error) {
	var state TGState
	err := r.db.GetContext(ctx, &state, "SELECT * FROM tg_state WHERE integration_id = ?", integrationID)
	return &state, err
}

func (r *StateRepository) UpsertTGState(ctx context.Context, state TGState) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tg_state (integration_id, pts, qts, seq, date) 
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(integration_id) DO UPDATE SET 
			pts = excluded.pts, 
			qts = excluded.qts, 
			seq = excluded.seq, 
			date = excluded.date`,
		state.IntegrationID, state.Pts, state.Qts, state.Seq, state.Date)
	return err
}

func (r *StateRepository) GetChannelPts(ctx context.Context, integrationID, channelID int64) (int, error) {
	var pts int
	err := r.db.GetContext(ctx, &pts, "SELECT pts FROM tg_channels WHERE integration_id = ? AND channel_id = ?", integrationID, channelID)
	return pts, err
}

func (r *StateRepository) UpsertChannelPts(ctx context.Context, integrationID, channelID int64, pts int) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tg_channels (integration_id, channel_id, pts) 
		VALUES (?, ?, ?)
		ON CONFLICT(integration_id, channel_id) DO UPDATE SET pts = excluded.pts`,
		integrationID, channelID, pts)
	return err
}

func (r *StateRepository) GetAllChannels(ctx context.Context, integrationID int64) ([]struct {
	ChannelID int64 `db:"channel_id"`
	Pts       int   `db:"pts"`
}, error) {
	var channels []struct {
		ChannelID int64 `db:"channel_id"`
		Pts       int   `db:"pts"`
	}
	err := r.db.SelectContext(ctx, &channels, "SELECT channel_id, pts FROM tg_channels WHERE integration_id = ?", integrationID)
	return channels, err
}
