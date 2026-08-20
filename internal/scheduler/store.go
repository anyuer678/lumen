package scheduler

import (
	"database/sql"
	"fmt"
)

// Store 调度器存储
type Store struct {
	db *sql.DB
}

// NewStore 创建存储
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// SaveJob 保存任务
func (s *Store) SaveJob(job *Job) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO jobs 
		(id, name, trigger_type, cron_expr, interval_secs, watch_path, goal_template, 
		 priority, enabled, catch_up, concurrency, last_run_at, next_run_at, last_status, miss_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Name, job.TriggerType, job.CronExpr, job.IntervalSecs, job.WatchPath,
		job.GoalTemplate, job.Priority, job.Enabled, job.CatchUp, job.Concurrency,
		job.LastRunAt, job.NextRunAt, job.LastStatus, job.MissCount, job.CreatedAt,
	)
	return err
}

// GetJob 获取任务
func (s *Store) GetJob(id string) (*Job, error) {
	job := &Job{}
	var lastRunAt, nextRunAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT id, name, trigger_type, cron_expr, interval_secs, watch_path, goal_template,
		       priority, enabled, catch_up, concurrency, last_run_at, next_run_at, last_status, miss_count, created_at
		FROM jobs WHERE id = ?`, id).Scan(
		&job.ID, &job.Name, &job.TriggerType, &job.CronExpr, &job.IntervalSecs, &job.WatchPath,
		&job.GoalTemplate, &job.Priority, &job.Enabled, &job.CatchUp, &job.Concurrency,
		&lastRunAt, &nextRunAt, &job.LastStatus, &job.MissCount, &job.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query job: %w", err)
	}

	if lastRunAt.Valid {
		job.LastRunAt = &lastRunAt.Time
	}
	if nextRunAt.Valid {
		job.NextRunAt = &nextRunAt.Time
	}

	return job, nil
}

// ListEnabledJobs 列出启用的任务
func (s *Store) ListEnabledJobs() ([]*Job, error) {
	rows, err := s.db.Query(`
		SELECT id, name, trigger_type, cron_expr, interval_secs, watch_path, goal_template,
		       priority, enabled, catch_up, concurrency, last_run_at, next_run_at, last_status, miss_count, created_at
		FROM jobs WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		job := &Job{}
		var lastRunAt, nextRunAt sql.NullTime

		if err := rows.Scan(
			&job.ID, &job.Name, &job.TriggerType, &job.CronExpr, &job.IntervalSecs, &job.WatchPath,
			&job.GoalTemplate, &job.Priority, &job.Enabled, &job.CatchUp, &job.Concurrency,
			&lastRunAt, &nextRunAt, &job.LastStatus, &job.MissCount, &job.CreatedAt,
		); err != nil {
			return nil, err
		}

		if lastRunAt.Valid {
			job.LastRunAt = &lastRunAt.Time
		}
		if nextRunAt.Valid {
			job.NextRunAt = &nextRunAt.Time
		}

		jobs = append(jobs, job)
	}
	return jobs, nil
}

// UpdateJob 更新任务
func (s *Store) UpdateJob(job *Job) error {
	_, err := s.db.Exec(`
		UPDATE jobs SET last_run_at = ?, next_run_at = ?, last_status = ?, miss_count = ?
		WHERE id = ?`,
		job.LastRunAt, job.NextRunAt, job.LastStatus, job.MissCount, job.ID,
	)
	return err
}

// DeleteJob 删除任务
func (s *Store) DeleteJob(id string) error {
	_, err := s.db.Exec("DELETE FROM jobs WHERE id = ?", id)
	return err
}

// ListJobs 列出所有任务
func (s *Store) ListJobs() ([]*Job, error) {
	rows, err := s.db.Query(`
		SELECT id, name, trigger_type, cron_expr, interval_secs, watch_path, goal_template,
		       priority, enabled, catch_up, concurrency, last_run_at, next_run_at, last_status, miss_count, created_at
		FROM jobs ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		job := &Job{}
		var lastRunAt, nextRunAt sql.NullTime

		if err := rows.Scan(
			&job.ID, &job.Name, &job.TriggerType, &job.CronExpr, &job.IntervalSecs, &job.WatchPath,
			&job.GoalTemplate, &job.Priority, &job.Enabled, &job.CatchUp, &job.Concurrency,
			&lastRunAt, &nextRunAt, &job.LastStatus, &job.MissCount, &job.CreatedAt,
		); err != nil {
			return nil, err
		}

		if lastRunAt.Valid {
			job.LastRunAt = &lastRunAt.Time
		}
		if nextRunAt.Valid {
			job.NextRunAt = &nextRunAt.Time
		}

		jobs = append(jobs, job)
	}
	return jobs, nil
}
