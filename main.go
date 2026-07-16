// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command github-dark-factory-watcher polls GitHub for open DRAFT pull requests
// that carry an approved-not-completed dark-factory spec in the PR diff and
// publishes a CreateTaskCommand to Kafka per (PR, head SHA) so the
// github-dark-factory-agent picks it up automatically.
package main

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	"github.com/bborbe/github-dark-factory-watcher/pkg"
	"github.com/bborbe/github-dark-factory-watcher/pkg/factory"
	"github.com/bborbe/github-dark-factory-watcher/pkg/filter"
	libhttp "github.com/bborbe/http"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/log"
	"github.com/bborbe/run"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	libtime "github.com/bborbe/time"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	lib "github.com/bborbe/maintainer"
	repoallowlist "github.com/bborbe/maintainer/repoallowlist"
)

var repoScopePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func validateLengthCaps(ctx context.Context, maxSlugLen, maxTitleLen int) error {
	if maxSlugLen <= 0 {
		return errors.Errorf(ctx, "MAX_SLUG_LEN must be > 0; got %d", maxSlugLen)
	}
	if maxTitleLen <= 0 {
		return errors.Errorf(ctx, "MAX_TITLE_LEN must be > 0; got %d", maxTitleLen)
	}
	if maxSlugLen >= maxTitleLen {
		return errors.Errorf(
			ctx,
			"MAX_SLUG_LEN (%d) must be < MAX_TITLE_LEN (%d)",
			maxSlugLen,
			maxTitleLen,
		)
	}
	return nil
}

func validateRepoScope(ctx context.Context, scope string) error {
	if !repoScopePattern.MatchString(scope) {
		return errors.Errorf(ctx, "repo scope %q must match ^[a-zA-Z0-9_.-]+$", scope)
	}
	return nil
}

// parseBackfillDuration parses raw as a libtime.Duration. Empty string returns 0 (disabled).
// Negative values are rejected.
func parseBackfillDuration(ctx context.Context, raw string) (libtime.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	parsed, err := libtime.ParseDuration(ctx, raw)
	if err != nil {
		return 0, errors.Wrapf(ctx, err, "parse BACKFILL_DURATION")
	}
	if parsed != nil && *parsed < 0 {
		return 0, errors.Errorf(ctx, "BACKFILL_DURATION must not be negative, got %s", *parsed)
	}
	if parsed == nil {
		return 0, nil
	}
	return *parsed, nil
}

func getEnvInt(name string) int64 {
	v, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy"`

	Listen           string           `required:"false" arg:"listen"            env:"LISTEN"            usage:"HTTP listen address (healthz/readiness/metrics)"                                                                              default:":9090"`
	AppID            int64            `required:"false" arg:"app-id"            env:"APP_ID"            usage:"GitHub App ID (numeric); requires InstallationID + PEMKey to be set for App auth"`
	InstallationID   int64            `required:"false" arg:"installation-id"   env:"INSTALLATION_ID"   usage:"GitHub App Installation ID (numeric)"`
	PEMKey           string           `required:"false" arg:"pem-key"           env:"PEM_KEY"           usage:"GitHub App private key (PEM content from k8s Secret envFrom)"                                                                                  display:"length"`
	KafkaBrokers     libkafka.Brokers `required:"true"  arg:"kafka-brokers"     env:"KAFKA_BROKERS"     usage:"Comma-separated Kafka broker list"`
	Stage            string           `required:"true"  arg:"stage"             env:"STAGE"             usage:"Deployment stage (dev|prod)"`
	PollInterval     string           `required:"false" arg:"poll-interval"     env:"POLL_INTERVAL"     usage:"Poll interval (Go duration)"                                                                                                  default:"5m"`
	RepoScope        string           `required:"false" arg:"repo-scope"        env:"REPO_SCOPE"        usage:"GitHub user/org scope"                                                                                                        default:"bborbe"`
	BackfillDuration string           `required:"false" arg:"backfill-duration" env:"BACKFILL_DURATION" usage:"On cold start, backdate the initial cursor by this duration (Go duration; empty disables)"                                    default:"720h"`
	RepoAllowlist    string           `required:"false" arg:"repo-allowlist"    env:"REPO_ALLOWLIST"    usage:"Comma-separated host-qualified repo allowlist (host/owner/repo format); empty means allow-all"`
	MaxSlugLen       int              `required:"false" arg:"max-slug-len"      env:"MAX_SLUG_LEN"      usage:"Max length of slugified PR-title segment in vault filenames"                                                                  default:"80"`
	MaxTitleLen      int              `required:"false" arg:"max-title-len"     env:"MAX_TITLE_LEN"     usage:"Max length of vault task filename (whole title; safety cap)"                                                                  default:"200"`
	TaskSuffix       string           `required:"false" arg:"task-suffix"       env:"TASK_SUFFIX"       usage:"Optional suffix appended to task filenames as ' - suffix'; empty = no suffix."`
	TargetVault      string           `required:"false" arg:"target-vault"      env:"TARGET_VAULT"      usage:"Vault slug stamped on every CreateTaskCommand (matched verbatim against the controller's VAULT_NAME). Empty leaves it unset."`

	// TopicPrefix selects the Kafka topic prefix used for CQRS topic construction
	// (e.g. "develop" / "master"); independent of Stage.
	TopicPrefix base.TopicPrefix `required:"false" arg:"topic-prefix" env:"TOPIC_PREFIX" usage:"Kafka topic prefix for CQRS topic construction"`

	TriggerHandler http.Handler
}

// resolveAuth determines the GitHub auth mode from environment variables and returns
// an authenticated *http.Client. App auth requires APP_ID + INSTALLATION_ID + PEM_KEY
// all set.
func (a *application) resolveAuth(ctx context.Context) (*http.Client, error) {
	appID := getEnvInt("APP_ID")
	installationID := getEnvInt("INSTALLATION_ID")
	pemKey := []byte(os.Getenv("PEM_KEY"))

	appPartial := (appID != 0) || (installationID != 0) || (len(pemKey) != 0)
	appComplete := (appID != 0) && (installationID != 0) && (len(pemKey) != 0)
	if appPartial && !appComplete {
		var missing []string
		if appID == 0 {
			missing = append(missing, "APP_ID")
		}
		if installationID == 0 {
			missing = append(missing, "INSTALLATION_ID")
		}
		if len(pemKey) == 0 {
			missing = append(missing, "PEM_KEY")
		}
		return nil, errors.Errorf(
			ctx,
			"watcher auth: partial GitHub App config — missing %v; set all three or none",
			missing,
		)
	}

	if !appComplete {
		return nil, errors.Errorf(
			ctx,
			"watcher auth: GitHub App credentials not configured — set APP_ID, INSTALLATION_ID, and PEM_KEY",
		)
	}
	glog.Infof(
		"watcher auth mode=github-app app_id=%d installation_id=%d",
		appID,
		installationID,
	)
	return factory.CreateGitHubAppClient(ctx, appID, installationID, pemKey)
}

func (a *application) validateConfig(ctx context.Context) error {
	if err := validateRepoScope(ctx, a.RepoScope); err != nil {
		return errors.Wrap(ctx, err, "validate repo scope")
	}
	if err := validateLengthCaps(ctx, a.MaxSlugLen, a.MaxTitleLen); err != nil {
		return errors.Wrap(ctx, err, "validate length caps")
	}
	return nil
}

//nolint:funlen // wires Run from validated config — extracting any chunk hurts readability without reducing complexity.
func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	if err := a.validateConfig(ctx); err != nil {
		return errors.Wrap(ctx, err, "validate config")
	}

	pollInterval, err := time.ParseDuration(a.PollInterval)
	if err != nil {
		return errors.Wrapf(ctx, err, "parse poll interval %q", a.PollInterval)
	}

	currentDateTime := libtime.NewCurrentDateTime()
	startTime := currentDateTime.Now()
	backfillDuration, err := parseBackfillDuration(ctx, a.BackfillDuration)
	if err != nil {
		return errors.Wrap(ctx, err, "parse backfill duration")
	}
	if backfillDuration > 0 {
		startTime = startTime.Add(-backfillDuration)
		glog.V(2).
			Infof("cursor cold-start backfilled by %s; initial=%s", backfillDuration, startTime)
	}

	repoAllowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
	if err != nil {
		return errors.Wrap(ctx, err, "parse repo allowlist")
	}
	if validationErr := repoallowlist.Validate(ctx, repoAllowlist); validationErr != nil {
		glog.Warningf("repo-allowlist: malformed entries ignored at match time: %v", validationErr)
	}
	if len(repoAllowlist) == 0 {
		glog.V(2).Infof("repo-allowlist count=0 (allow-all)")
	} else {
		glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))
	}
	scopeFilter := filter.TaskCreationFilters{
		filter.NewRepoAllowlistFilter(repoAllowlist),
	}

	syncProducer, err := libkafka.NewSyncProducerWithName(
		ctx,
		a.KafkaBrokers,
		"github-dark-factory-watcher",
	)
	if err != nil {
		return errors.Wrap(ctx, err, "create sync producer")
	}
	defer func() {
		if err := syncProducer.Close(); err != nil {
			glog.Warningf("close kafka sync producer: %v", err)
		}
	}()
	createSender := factory.CreateKafkaSender(syncProducer, a.TopicPrefix)

	httpClient, err := a.resolveAuth(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "resolve auth")
	}

	ghClient := pkg.NewGitHubClient(httpClient)
	metrics := pkg.NewMetrics(nil)

	taskCfg := pkg.TaskConfig{
		Stage:       a.Stage,
		MaxSlugLen:  a.MaxSlugLen,
		MaxTitleLen: a.MaxTitleLen,
		TaskSuffix:  a.TaskSuffix,
		TargetVault: a.TargetVault,
	}

	w := factory.CreateWatcher(
		ghClient,
		createSender,
		metrics,
		pkg.DefaultCursorPath,
		startTime,
		a.RepoScope,
		scopeFilter,
		taskCfg,
	)

	// HTTP-side sender backs the /trigger handler.
	triggerSender := factory.CreateTriggerCommandSender(
		ctx,
		syncProducer,
		a.TopicPrefix,
	)
	triggerHandler := factory.CreateSinglePRTriggerHandler(triggerSender)
	a.TriggerHandler = libhttp.NewJSONErrorHandler(triggerHandler)

	// In-pod command consumer: third run.Func alongside poll + HTTP.
	// Session-scoped offset store — replays the request topic from OffsetOldest
	// on pod restart; safe because the downstream CreateTaskCommand is
	// idempotent via the derived task_id.
	saramaClientProvider := libkafka.NewSaramaClientProviderNew(a.KafkaBrokers)
	db := pkg.NewMemDB()
	commandConsumer := factory.CreateCommandConsumer(
		saramaClientProvider,
		syncProducer,
		db,
		ghClient, // shared with the watcher
		createSender,
		taskCfg,
		metrics, // shared with the watcher
		a.TopicPrefix,
		currentDateTime, // time-injected clock for Force=true nonce
	)

	glog.V(2).
		Infof("github-dark-factory-watcher starting stage=%s scope=%s interval=%s listen=%s schema=%s", a.Stage, a.RepoScope, a.PollInterval, a.Listen, lib.GithubDarkFactoryV1SchemaID)

	pollOnce := a.pollOnce(w)

	// Order: poll → HTTP → command consumer (three run.Funcs).
	return run.CancelOnFirstFinish(ctx,
		a.runPollLoop(pollOnce, pollInterval),
		a.createHTTPServer(pollOnce),
		commandConsumer,
	)
}

func (a *application) pollOnce(w pkg.Watcher) run.Func {
	return func(ctx context.Context) error {
		glog.V(2).Infof("poll cycle start stage=%s", a.Stage)
		return w.Poll(ctx)
	}
}

func (a *application) runPollLoop(poll run.Func, interval time.Duration) run.Func {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				glog.V(2).Infof("poll loop: context cancelled, exiting cleanly")
				return nil
			case <-ticker.C:
				if err := poll(ctx); err != nil {
					glog.Errorf("poll cycle error: %v", err)
				}
			}
		}
	}
}

func (a *application) createHTTPServer(poll run.Func) run.Func {
	return func(ctx context.Context) error {
		router := mux.NewRouter()
		router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
		router.Path("/metrics").Handler(promhttp.Handler())
		router.Path("/setloglevel/{level}").
			Handler(log.NewSetLoglevelHandler(ctx, log.NewLogLevelSetter(2, 5*time.Minute)))
		router.Path("/check").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))
		router.Path("/trigger").Handler(a.TriggerHandler)
		glog.V(2).Infof("http server listening on %s", a.Listen)
		return libhttp.NewServer(a.Listen, router).Run(ctx)
	}
}
