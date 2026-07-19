package main

import (
	"time"

	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services/reporting/eventbus"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/otel"
	"github.com/sysadminsmedia/homebox/backend/pkgs/mailer"
)

type app struct {
	conf                 *config.Config
	mailer               mailer.Mailer
	db                   *ent.Client
	repos                *repo.AllRepos
	services             *services.AllServices
	bus                  *eventbus.EventBus
	authLimiter          *authRateLimiter
	registrationLimiter  *simpleRateLimiter
	passwordResetLimiter *simpleRateLimiter
	notifierTestLimiter  *simpleRateLimiter
	otel                 *otel.Provider
}

func new(conf *config.Config) *app {
	s := &app{
		conf: conf,
	}

	s.mailer = mailer.Mailer{
		Host:     s.conf.Mailer.Host,
		Port:     s.conf.Mailer.Port,
		Username: s.conf.Mailer.Username,
		Password: s.conf.Mailer.Password,
		From:     s.conf.Mailer.From,
	}

	s.authLimiter = newAuthRateLimiter(s.conf.Auth.RateLimit)
	if s.authLimiter.cfg.Enabled {
		// Registration must count successful requests as well as failures. The
		// authentication limiter clears its state after a successful handler,
		// so it cannot protect account-creation work.
		s.registrationLimiter = newSimpleRateLimiter(
			s.authLimiter.cfg.MaxAttempts,
			s.authLimiter.cfg.Window,
			s.conf.Options.TrustProxy,
		)

		// Password-reset requests intentionally return 204 whether or not an
		// account exists. A dedicated all-outcomes limiter is required because
		// the login limiter clears its state after successful handlers.
		s.passwordResetLimiter = newSimpleRateLimiter(
			s.authLimiter.cfg.MaxAttempts,
			s.authLimiter.cfg.Window,
			s.conf.Options.TrustProxy,
		)
	}
	s.notifierTestLimiter = newSimpleRateLimiter(10, time.Minute, s.conf.Options.TrustProxy) // 10 requests per minute

	return s
}

func (a *app) stopRateLimiters() {
	if a.authLimiter != nil {
		a.authLimiter.Stop()
	}
	if a.registrationLimiter != nil {
		a.registrationLimiter.Stop()
	}
	if a.passwordResetLimiter != nil {
		a.passwordResetLimiter.Stop()
	}
	if a.notifierTestLimiter != nil {
		a.notifierTestLimiter.Stop()
	}
}
