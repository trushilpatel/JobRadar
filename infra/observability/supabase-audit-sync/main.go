// Supabase auth.audit_log_entries -> Loki shipper.
//
// On each interval, pulls new rows since the last watermark (persisted in
// STATE_FILE) and pushes them to Loki as JSON log lines tagged
// service="supabase_auth".
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type config struct {
	DBURL         string
	LokiURL       string
	Interval      time.Duration
	StateFile     string
	BatchSize     int
	HTTPTimeout   time.Duration
}

type state struct {
	LastCreatedAt time.Time `json:"last_created_at"`
}

type auditRow struct {
	ID        string
	CreatedAt time.Time
	Payload   []byte
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

type lokiPush struct {
	Streams []lokiStream `json:"streams"`
}

func loadConfig() (config, error) {
	var c config
	c.DBURL = os.Getenv("SUPABASE_DB_URL")
	if c.DBURL == "" {
		return c, errors.New("SUPABASE_DB_URL is required")
	}
	c.LokiURL = os.Getenv("LOKI_URL")
	if c.LokiURL == "" {
		return c, errors.New("LOKI_URL is required")
	}
	intervalSec := 900
	if v := os.Getenv("AUDIT_SYNC_INTERVAL_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c, fmt.Errorf("invalid AUDIT_SYNC_INTERVAL_SECONDS: %q", v)
		}
		intervalSec = n
	}
	c.Interval = time.Duration(intervalSec) * time.Second
	c.StateFile = os.Getenv("STATE_FILE")
	if c.StateFile == "" {
		c.StateFile = "/var/lib/audit-sync/state.json"
	}
	c.BatchSize = 500
	c.HTTPTimeout = 30 * time.Second
	return c, nil
}

func loadState(path string) (state, error) {
	var s state
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Cold start: pull everything from "now-24h" so we get recent context.
			s.LastCreatedAt = time.Now().UTC().Add(-24 * time.Hour)
			return s, nil
		}
		return s, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parse state: %w", err)
	}
	return s, nil
}

func saveState(path string, s state) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

func fetchRows(ctx context.Context, pool *pgxpool.Pool, after time.Time, limit int) ([]auditRow, error) {
	const q = `
		SELECT id::text, created_at, payload::text
		FROM auth.audit_log_entries
		WHERE created_at > $1
		ORDER BY created_at ASC
		LIMIT $2
	`
	rows, err := pool.Query(ctx, q, after, limit)
	if err != nil {
		return nil, fmt.Errorf("query audit log: %w", err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		var payload string
		if err := rows.Scan(&r.ID, &r.CreatedAt, &payload); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		r.Payload = []byte(payload)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return out, nil
}

func pushToLoki(ctx context.Context, client *http.Client, url string, rows []auditRow) error {
	if len(rows) == 0 {
		return nil
	}
	values := make([][2]string, 0, len(rows))
	for _, r := range rows {
		ts := strconv.FormatInt(r.CreatedAt.UnixNano(), 10)
		// Line is the raw JSON payload; Loki will index by stream label.
		values = append(values, [2]string{ts, string(r.Payload)})
	}
	body := lokiPush{
		Streams: []lokiStream{{
			Stream: map[string]string{
				"service": "supabase_auth",
				"source":  "audit_log_entries",
			},
			Values: values,
		}},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal push: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("loki returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func runOnce(ctx context.Context, cfg config, pool *pgxpool.Pool, httpClient *http.Client) error {
	st, err := loadState(cfg.StateFile)
	if err != nil {
		return err
	}
	for {
		rows, err := fetchRows(ctx, pool, st.LastCreatedAt, cfg.BatchSize)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		if err := pushToLoki(ctx, httpClient, cfg.LokiURL, rows); err != nil {
			return err
		}
		st.LastCreatedAt = rows[len(rows)-1].CreatedAt
		if err := saveState(cfg.StateFile, st); err != nil {
			return err
		}
		if len(rows) < cfg.BatchSize {
			return nil
		}
	}
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DBURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}

	log.Printf("supabase-audit-sync started (interval=%s, state=%s)", cfg.Interval, cfg.StateFile)

	// Run once immediately, then on the ticker.
	if err := runOnce(ctx, cfg, pool, httpClient); err != nil {
		log.Printf("initial run failed: %v", err)
	}

	t := time.NewTicker(cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case <-t.C:
			if err := runOnce(ctx, cfg, pool, httpClient); err != nil {
				log.Printf("sync failed: %v", err)
			}
		}
	}
}
