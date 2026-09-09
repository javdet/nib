package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

// planOwnershipMigrationVersion is recorded in schema_migrations so the file move
// below runs at most once, the same way the .up.sql files are tracked.
const planOwnershipMigrationVersion = "000021_plan_files_to_decompose_dialog"

// actionPlanFileSuffixes are the files an action plan is spread across, all keyed
// by the id of the dialog that owns the plan.
var actionPlanFileSuffixes = []string{
	".json",
	".checks.json",
	".comments.json",
	".runs.json",
	".executor.json",
}

// migratePlanOwnership moves action plan files off the child "plan" dialog that
// used to own them and onto its decompose parent, which owns the plan now that
// stages are written by a fan-out of subagents instead of by one plan dialog.
// Plan state and the DAG were always keyed by the parent, so only these files move.
//
// A parent that already holds a plan is left alone, so the move never overwrites
// newer content and is safe if the version row is ever cleared.
func migratePlanOwnership(ctx context.Context, pool *pgxpool.Pool, dataDir string) error {
	var applied bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`,
		planOwnershipMigrationVersion,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check plan ownership migration: %w", err)
	}
	if applied {
		return nil
	}

	rows, err := pool.Query(ctx,
		`SELECT id::text, parent_id::text FROM chat_dialogs WHERE mode = 'plan' AND parent_id IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("list plan dialogs: %w", err)
	}
	defer rows.Close()

	type lineage struct{ child, parent string }
	var pairs []lineage
	for rows.Next() {
		var l lineage
		if err := rows.Scan(&l.child, &l.parent); err != nil {
			return fmt.Errorf("scan plan dialog: %w", err)
		}
		pairs = append(pairs, l)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate plan dialogs: %w", err)
	}

	dir := filepath.Join(dataDir, "action_plans")
	moved := 0
	for _, l := range pairs {
		// The plan file itself decides whether this pair has anything to move:
		// the checkbox, comment and run side files are meaningless without it.
		if _, err := os.Stat(filepath.Join(dir, l.child+".json")); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, l.parent+".json")); err == nil {
			slog.Warn("plan ownership: parent already holds a plan, leaving the child untouched",
				"child_dialog_id", l.child, "parent_dialog_id", l.parent)
			continue
		}

		for _, suffix := range actionPlanFileSuffixes {
			from := filepath.Join(dir, l.child+suffix)
			if _, err := os.Stat(from); err != nil {
				continue
			}
			if err := os.Rename(from, filepath.Join(dir, l.parent+suffix)); err != nil {
				return fmt.Errorf("move %s: %w", from, err)
			}
		}
		moved++
	}

	// DO NOTHING rather than a bare insert: the file moves above are not
	// transactional, so an overlapping pod (or a crash between the moves and
	// this row) must not turn a re-run into a primary key crash on boot.
	if _, err := pool.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING`,
		planOwnershipMigrationVersion,
	); err != nil {
		return fmt.Errorf("record plan ownership migration: %w", err)
	}

	slog.Info("plan ownership migration applied", "plans_moved", moved, "plan_dialogs", len(pairs))
	return nil
}
