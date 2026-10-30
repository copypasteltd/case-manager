package main

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/notification"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/migrations"
	"crypto/rand"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

func notificationContractRule() (notification.ContractRule, error) {
	rule := notification.ContractRule{OrgID: os.Getenv("CP_NOTIFICATION_CONTRACT_ORG_ID"), Timezone: os.Getenv("CP_NOTIFICATION_CONTRACT_TIMEZONE")}
	if raw := os.Getenv("CP_NOTIFICATION_CONTRACT_DAYS_BEFORE"); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			days, err := strconv.Atoi(strings.TrimSpace(item))
			if err != nil {
				return rule, &startupError{"invalid notification contract advance-day configuration"}
			}
			rule.DaysBefore = append(rule.DaysBefore, days)
		}
	}
	if err := rule.Validate(); err != nil {
		return rule, &startupError{"notification contract reminders require an explicit organization, timezone and distinct advance days"}
	}
	return rule, nil
}
func notificationRetry(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 4 {
		attempt = 4
	}
	var noise [1]byte
	_, _ = rand.Read(noise[:])
	return time.Duration((1<<uint(attempt-1))*15+int(noise[0])%11) * time.Second
}
func runNotificationWorker(ctx context.Context, logger *slog.Logger) error {
	config, err := platform.LoadWorkerConfig()
	if err != nil {
		return &startupError{err.Error()}
	}
	rule, err := notificationContractRule()
	if err != nil {
		return err
	}
	db, err := platform.OpenDatabase(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = migrations.CheckCompiled(ctx, db); err != nil {
		return &startupError{"notification worker requires matching database migrations"}
	}
	service := &notification.Service{DB: db}
	queue := jobqueue.Queue{DB: db}
	owner := "notification." + core.NewID()
	consumer := notification.Consumer()
	var nextSchedule time.Time
	var nextPolicyOrg string
	var nextResourceSchedule time.Time
	var nextResourceSource string
	logger.Info("notification worker started", "owner", owner, "contract_reminders_enabled", len(rule.DaysBefore) > 0)
	for ctx.Err() == nil {
		if time.Now().After(nextResourceSchedule) {
			if next, _, e := service.ScheduleResources(ctx, nextResourceSource); e != nil && ctx.Err() == nil {
				logger.Warn("resource reminder scheduling failed", "code", core.PublicError(e).Code)
			} else if e == nil {
				nextResourceSource = next
			}
			nextResourceSchedule = time.Now().Add(10 * time.Second)
		}
		if time.Now().After(nextSchedule) {
			if next, _, e := service.ScheduleContractPolicies(ctx, nextPolicyOrg); e != nil && ctx.Err() == nil {
				logger.Warn("published contract reminder scheduling failed", "code", core.PublicError(e).Code)
			} else if e == nil {
				nextPolicyOrg = next
			}
			if _, e := service.ScheduleContracts(ctx, rule); e != nil && ctx.Err() == nil {
				logger.Warn("contract reminder scheduling failed", "code", core.PublicError(e).Code)
			}
			if _, e := queue.SweepExhausted(ctx, []string{notification.JobKind}, "notification"); e != nil && ctx.Err() == nil {
				logger.Warn("notification sweep failed", "code", core.PublicError(e).Code)
			}
			nextSchedule = time.Now().Add(time.Minute)
		}
		for i := 0; i < 8; i++ {
			delivery, e := queue.DispatchOne(ctx, consumer)
			if e != nil {
				if ctx.Err() == nil {
					logger.Warn("notification dispatch failed", "code", core.PublicError(e).Code)
				}
				break
			}
			if delivery == nil {
				break
			}
		}
		job, e := queue.Claim(ctx, []string{notification.JobKind}, "notification", owner, 2*time.Minute)
		if e != nil && ctx.Err() == nil {
			logger.Warn("notification claim failed", "code", core.PublicError(e).Code)
		}
		if job != nil {
			result, e2 := service.Process(ctx, *job)
			if e2 != nil && ctx.Err() == nil {
				code := core.PublicError(e2).Code
				permanent := core.PublicError(e2).Status == 422 || core.PublicError(e2).Status == 404
				if e3 := queue.Fail(ctx, *job, code, permanent, notificationRetry(job.Attempt)); e3 != nil {
					logger.Warn("notification failure receipt rejected", "code", core.PublicError(e3).Code)
				}
				logger.Warn("notification delivery failed", "job_id", job.ID, "code", code)
			} else if e2 == nil {
				logger.Info("notification delivery recorded", "job_id", job.ID, "delivered", result.Delivered, "suppressed", result.Suppressed)
			}
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}
