package newdb

import (
	"context"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/chat-migration-cli/internal/model/new"
)

type ProviderStore struct {
	db *DB
}

func NewProviderStore(db *DB) *ProviderStore {
	return &ProviderStore{db: db}
}

// -------------------------
// MetaApp
// -------------------------

func (s *ProviderStore) InsertMetaApps(ctx context.Context, tx pgx.Tx, apps []*new.MetaApp) error {
	query := squirrel.Insert("im_provider.meta_apps").Columns(
		"id",
		"name",
		"app_id",
		"app_secret",
		"redirect_uri",
		"uri",
		"scopes",
		"created_at",
		"updated_at",
		"verify_token",
	).PlaceholderFormat(squirrel.Dollar)

	for _, a := range apps {
		query = query.Values(
			a.ID,
			a.Name,
			a.AppID,
			a.AppSecret,
			a.RedirectURI,
			a.URI,
			a.Scopes,
			a.CreatedAt,
			a.UpdatedAt,
			a.VerifyToken,
		)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, sql, args...)
	return err
}

// -------------------------
// GateWABA
// -------------------------

func (s *ProviderStore) InsertGateWABAs(ctx context.Context, tx pgx.Tx, gates []*new.GateWABA) error {
	query := squirrel.Insert("im_provider.gate_waba").Columns(
		"id",
		"meta_app_id",
		"phone_number",
		"phone_number_id",
		"access_token",
		"access_token_expires_at",
		"business_id",
	).PlaceholderFormat(squirrel.Dollar)

	for _, g := range gates {
		query = query.Values(
			g.ID,
			g.MetaAppID,
			g.PhoneNumber,
			g.PhoneNumberID,
			g.AccessToken,
			g.AccessTokenExpiresAt,
			g.BusinessID,
		)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, sql, args...)
	return err
}

// -------------------------
// Facebook
// -------------------------

func (s *ProviderStore) InsertFacebooks(ctx context.Context, tx pgx.Tx, pages []*new.Facebook) error {
	query := squirrel.Insert("im_provider.facebook").Columns(
		"gate_id",
		"meta_app_id",
		"page_id",
		"page_token",
	).PlaceholderFormat(squirrel.Dollar)

	for _, f := range pages {
		query = query.Values(
			f.GateID,
			f.MetaAppID,
			f.PageID,
			f.PageToken,
		)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, sql, args...)
	return err
}

// -------------------------
// Bot
// -------------------------

func (s *ProviderStore) InsertBots(ctx context.Context, tx pgx.Tx, bots []*new.Bot) error {
	query := squirrel.Insert("im_provider.bots").Columns(
		"id",
		"sub",
		"iss",
		"gate_id",
		"created_at",
	).PlaceholderFormat(squirrel.Dollar)

	for _, b := range bots {
		query = query.Values(
			b.ID,
			b.Sub,
			b.Iss,
			b.GateID,
			b.CreatedAt,
		)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, sql, args...)
	return err
}
