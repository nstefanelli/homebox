package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/containrrr/shoutrrr"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
)

type Latest struct {
	Version string `json:"version"`
	Date    string `json:"date"`
}

const (
	defaultGithubReleaseURL = "https://api.github.com/repos/nstefanelli/homebox/releases/latest"
	releaseCheckTimeout     = 10 * time.Second
	maxReleaseResponseSize  = 1 << 20
)

type BackgroundService struct {
	repos          *repo.AllRepos
	latestMu       sync.RWMutex
	latest         Latest
	notifierConfig *config.NotifierConf
	releaseURL     string
	httpClient     *http.Client
}

func (svc *BackgroundService) SendNotifiersToday(ctx context.Context) error {
	// Get All Groups
	groups, err := svc.repos.Groups.GetAllGroups(ctx, uuid.Nil)
	if err != nil {
		return err
	}

	today := types.DateFromTime(time.Now())

	for i := range groups {
		group := groups[i]

		entries, err := svc.repos.MaintEntry.GetScheduled(ctx, group.ID, today)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			log.Debug().
				Str("group_name", group.Name).
				Str("group_id", group.ID.String()).
				Msg("No scheduled maintenance for today")
			continue
		}

		notifiers, err := svc.repos.Notifiers.GetActiveByGroup(ctx, group.ID)
		if err != nil {
			return err
		}

		if len(notifiers) == 0 {
			log.Debug().
				Str("group_name", group.Name).
				Str("group_id", group.ID.String()).
				Msg("No active notifiers configured")
			continue
		}

		bldr := strings.Builder{}

		bldr.WriteString("Homebox Maintenance for (")
		bldr.WriteString(today.String())
		bldr.WriteString("):\n")

		for i := range entries {
			entry := entries[i]
			bldr.WriteString(" - ")
			bldr.WriteString(entry.Name)
			bldr.WriteString("\n")
		}

		var sendErrs []error
		for i := range notifiers {
			// Validate notifier URL before sending
			if err := validate.ValidateNotifierURL(notifiers[i].URL, svc.notifierConfig); err != nil {
				log.Error().
					Err(err).
					Str("notifier_id", notifiers[i].ID.String()).
					Str("notifier_name", notifiers[i].Name).
					Msg("notifier URL failed validation, skipping")
				sendErrs = append(sendErrs, fmt.Errorf("notifier %s failed validation: %w", notifiers[i].Name, err))
				continue
			}

			err := shoutrrr.Send(notifiers[i].URL, bldr.String())

			if err != nil {
				sendErrs = append(sendErrs, err)
			}
		}

		if len(sendErrs) > 0 {
			return sendErrs[0]
		}
	}

	return nil
}

func (svc *BackgroundService) GetLatestGithubRelease(ctx context.Context) error {
	releaseURL := svc.releaseURL
	if releaseURL == "" {
		releaseURL = defaultGithubReleaseURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create latest version request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Homebox-Fork-Version-Checker")

	client := svc.httpClient
	if client == nil {
		client = &http.Client{Timeout: releaseCheckTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make latest version request: %w", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Printf("error closing latest version response body: %v", err)
		}
	}()

	// This fork does not currently publish GitHub releases. Treat a missing
	// latest release as an expected empty state so opting into the checker
	// does not produce an hourly error until the first fork release exists.
	if resp.StatusCode == http.StatusNotFound {
		svc.setLatestVersion(Latest{})
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("latest version unexpected status code: %d", resp.StatusCode)
	}

	// ignoring fields that are not relevant
	type Release struct {
		ReleaseVersion string    `json:"tag_name"`
		PublishedAt    time.Time `json:"published_at"`
	}

	var release Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseResponseSize)).Decode(&release); err != nil {
		return fmt.Errorf("failed to decode latest version response: %w", err)
	}

	svc.setLatestVersion(Latest{
		Version: release.ReleaseVersion,
		Date:    release.PublishedAt.String(),
	})

	return nil
}

func (svc *BackgroundService) GetLatestVersion() Latest {
	svc.latestMu.RLock()
	defer svc.latestMu.RUnlock()
	return svc.latest
}

func (svc *BackgroundService) setLatestVersion(latest Latest) {
	svc.latestMu.Lock()
	defer svc.latestMu.Unlock()
	svc.latest = latest
}
