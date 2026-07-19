package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/config"
)

const (
	// demoPasswordEnv is the env var operators set when running demo mode
	// outside development, overriding the hardcoded development default below.
	demoPasswordEnv = "HBOX_DEMO_PASSWORD" // #nosec G101 -- an environment variable name, not a credential value.
	// demoPasswordDefault is public knowledge and is therefore only available
	// while running explicitly in development mode.
	demoPasswordDefault = "demodemo"
	// demoPasswordMinLength is the minimum acceptable length for a demo
	// password outside development mode.
	demoPasswordMinLength = 12
)

// resolveDemoPassword keeps the public default available for local development
// without allowing it to become an implicit credential on a deployed instance.
// Unknown modes fail closed too: a misspelled production mode must not restore
// the development password.
func resolveDemoPassword(mode, configured string) (string, error) {
	if mode == config.ModeDevelopment {
		if configured == "" {
			return demoPasswordDefault, nil
		}
		return configured, nil
	}

	if len(configured) < demoPasswordMinLength {
		return "", fmt.Errorf(
			"refusing to enable demo mode outside development: %s must be set to at least %d characters",
			demoPasswordEnv,
			demoPasswordMinLength,
		)
	}

	return configured, nil
}

func validateDemoConfig(cfg *config.Config) error {
	if !cfg.Demo {
		return nil
	}

	_, err := resolveDemoPassword(cfg.Mode, os.Getenv(demoPasswordEnv))
	return err
}

func (a *app) SetupDemo() error {
	csvText := `HB.import_ref,HB.location,HB.tags,HB.quantity,HB.name,HB.description,HB.insured,HB.serial_number,HB.model_number,HB.manufacturer,HB.notes,HB.purchase_from,HB.purchase_price,HB.purchase_date,HB.lifetime_warranty,HB.warranty_expires,HB.warranty_details,HB.sold_to,HB.sold_price,HB.sold_date,HB.sold_notes
,Garage,IOT;Home Assistant; Z-Wave,1,Zooz Universal Relay ZEN17,"Zooz 700 Series Z-Wave Universal Relay ZEN17 for Awnings, Garage Doors, Sprinklers, and More | 2 NO-C-NC Relays (20A, 10A) | Signal Repeater | Hub Required (Compatible with SmartThings and Hubitat)",,,ZEN17,Zooz,,Amazon,39.95,10/13/2021,,,,,,,
,Living Room,IOT;Home Assistant; Z-Wave,1,Zooz Motion Sensor,"Zooz Z-Wave Plus S2 Motion Sensor ZSE18 with Magnetic Mount, Works with Vera and SmartThings",,,ZSE18,Zooz,,Amazon,29.95,10/15/2021,,,,,,,
,Office,IOT; Home Assistant; Z-Wave,1,Zooz 110v Power Switch,"Zooz Z-Wave Plus Power Switch ZEN15 for 110V AC Units, Sump Pumps, Humidifiers, and More",,,ZEN15,Zooz,,Amazon,39.95,10/13/2021,,,,,,,
,Downstairs,IOT;Home Assistant; Z-Wave,1,Ecolink Z-Wave PIR Motion Sensor,"Ecolink Z-Wave PIR Motion Detector Pet Immune, White (PIRZWAVE2.5-ECO)",,,PIRZWAVE2.5-ECO,Ecolink,,Amazon,35.58,10/21/2020,,,,,,,
,Entry,IOT;Home Assistant; Z-Wave,1,Yale Security Touchscreen Deadbolt,"Yale Security YRD226-ZW2-619 YRD226ZW2619 Touchscreen Deadbolt, Satin Nickel",,,YRD226ZW2619,Yale,,Amazon,120.39,10/14/2020,,,,,,,
,Kitchen,IOT;Home Assistant; Z-Wave,1,Smart Rocker Light Dimmer,"UltraPro Z-Wave Smart Rocker Light Dimmer with QuickFit and SimpleWire, 3-Way Ready, Compatible with Alexa, Google Assistant, ZWave Hub Required, Repeater/Range Extender, White Paddle Only, 39351",,,39351,Honeywell,,Amazon,65.98,09/30/0202,,,,,,,
`

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	demoPassword, err := resolveDemoPassword(a.conf.Mode, os.Getenv(demoPasswordEnv))
	if err != nil {
		return err
	}

	registration := services.UserRegistration{
		Email:    "demo@example.com",
		Name:     "Demo",
		Password: demoPassword,
	}

	// If demo user already exists, skip all demo seeding tasks
	if a.services.User.ExistsByEmail(ctx, registration.Email) {
		log.Info().Msg("Demo user already exists; skipping demo seeding")
		return nil
	}

	// Otherwise, register the demo user. Production credentials have already
	// passed the stronger demoPasswordMinLength check. Keep the bypass here for
	// legacy development fixtures that intentionally use shorter passwords;
	// public registration still enforces services.PasswordMinLength.
	log.Debug().Msg("Registering demo user")
	_, err = a.services.User.RegisterUser(ctx, registration, services.SkipPasswordValidation())
	if err != nil {
		if ent.IsConstraintError(err) {
			// Concurrent creation race: treat as exists and skip
			log.Info().Msg("Demo user concurrently created; skipping seeding")
			return nil
		}
		log.Err(err).Msg("Failed to register demo user")
		return errors.New("failed to setup demo")
	}

	// Login the demo user to get a token
	token, err := a.services.User.Login(ctx, registration.Email, registration.Password, false)
	if err != nil {
		log.Err(err).Msg("Failed to login demo user")
		return errors.New("failed to setup demo")
	}
	self, err := a.services.User.GetSelf(ctx, token.Raw)
	if err != nil {
		log.Err(err).Msg("Failed to get self")
		return errors.New("failed to setup demo")
	}

	_, err = a.services.Entities.CsvImport(ctx, self.DefaultGroupID, strings.NewReader(csvText))
	if err != nil {
		log.Err(err).Msg("Failed to import CSV")
		return errors.New("failed to setup demo")
	}

	log.Info().Msg("Demo setup complete")

	return nil
}
