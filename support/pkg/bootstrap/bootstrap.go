// Package bootstrap wires the support service: repositories, the router, the
// Telegram adapter, HTTP routes, and the inbound update path.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/elug3/dupli1/shared/pkg/authjwt"
	"github.com/elug3/dupli1/shared/pkg/natspublisher"
	tg "github.com/elug3/dupli1/shared/pkg/telegram"
	"github.com/elug3/dupli1/support/pkg/domain"
	"github.com/elug3/dupli1/support/pkg/handler"
	emailinfra "github.com/elug3/dupli1/support/pkg/infra/email"
	gatewayinfra "github.com/elug3/dupli1/support/pkg/infra/gateway"
	"github.com/elug3/dupli1/support/pkg/infra/memory"
	natsinfra "github.com/elug3/dupli1/support/pkg/infra/nats"
	"github.com/elug3/dupli1/support/pkg/infra/postgres"
	telegraminfra "github.com/elug3/dupli1/support/pkg/infra/telegram"
	"github.com/elug3/dupli1/support/pkg/livefeed"
	"github.com/elug3/dupli1/support/pkg/ports"
	"github.com/elug3/dupli1/support/pkg/service"
)

// App holds the wired service.
type App struct {
	Router        *http.ServeMux
	HTTP          *http.Server
	cancelWorkers context.CancelFunc
	close         func() error
}

// Close releases infrastructure resources opened during bootstrap.
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	if a.cancelWorkers != nil {
		a.cancelWorkers()
	}
	if a.close == nil {
		return nil
	}
	return a.close()
}

// Bootstrap wires the support service and starts its inbound update path.
func Bootstrap(cfg Config) (*App, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("listen address is required")
	}
	// Fail closed at boot rather than at delivery time: SetWebhook without a
	// secret registers a webhook Telegram calls with no
	// X-Telegram-Bot-Api-Secret-Token header, which the handler then answers
	// with 503 for every update — a silently dead inbound path.
	if strings.TrimSpace(cfg.TelegramWebhookURL) != "" && strings.TrimSpace(cfg.TelegramWebhookSecret) == "" {
		return nil, fmt.Errorf("TELEGRAM_SUPPORT_WEBHOOK_SECRET is required when TELEGRAM_SUPPORT_WEBHOOK_URL is set")
	}

	store, err := openStore(cfg.DatabaseConnString)
	if err != nil {
		return nil, err
	}

	client := newBotClient(cfg)
	// No access policy is set, and that is the point: this bot answers whoever
	// writes to it. The ops bot's allowlist is the opposite posture and stays
	// with the ops bot.
	bot := &telegraminfra.Bot{Client: client}

	publisher, closePublisher, err := openPublisher(cfg)
	if err != nil {
		_ = store.close()
		return nil, err
	}

	// Live changes reach this replica's streams through the hub. With NATS
	// they go via the bus first, so a reply saved on one replica reaches a
	// shopper streaming from another; without it the hub is fed directly.
	hub := livefeed.NewHub()
	live, subscriber, closeLive, err := openLive(cfg, hub)
	if err != nil {
		_ = closePublisher()
		_ = store.close()
		return nil, err
	}
	closePublisher = joinClose(closeLive, closePublisher)

	router := service.NewRouter(service.Deps{
		Conversations: store.conversations,
		Answers:       store.answers,
		Inquiries:     store.inquiries,
		Messages:      store.messages,
		Publisher:     publisher,
		Live:          live,
		Bot:           bot,
		Hours:         cfg.BusinessHours,
		NewID:         newULID,
		Now:           time.Now,
	})
	processor := &telegraminfra.UpdateProcessor{Router: router}

	// Long-lived worker root; cancelled on shutdown. Created before the handler
	// because acknowledged webhook updates are processed under it, past the
	// lifetime of their request.
	workerCtx, cancelWorkers := context.WithCancel(context.Background())

	products, orders := openReferenceReaders(cfg)
	inbox := service.NewInbox(store.conversations, store.inquiries, store.messages, bot, newULID, time.Now).
		WithWebChat(products, orders, live)

	notifier, chatURL, err := openNotifier(cfg)
	if err != nil {
		cancelWorkers()
		_ = closePublisher()
		_ = store.close()
		return nil, err
	}
	webChat := service.NewWebChat(service.WebChatDeps{
		Conversations: store.conversations,
		Inquiries:     store.inquiries,
		Messages:      store.messages,
		Publisher:     publisher,
		Live:          live,
		Products:      products,
		Orders:        orders,
		Notifier:      notifier,
		Hours:         cfg.BusinessHours,
		ChatURL:       chatURL,
		NewID:         newULID,
		Now:           time.Now,
	})

	questions := service.NewProductQuestions(service.ProductQuestionsDeps{
		Repo:          store.questions,
		Products:      products,
		Publisher:     publisher,
		Notifier:      notifier,
		StorefrontURL: cfg.StorefrontURL,
		NewID:         newULID,
		Now:           time.Now,
	})

	// A deleted account takes its consultation words and its product
	// questions with it. Queue-grouped, so one replica does each deletion.
	forget := func(ctx context.Context, customerID string) error {
		return errors.Join(webChat.ForgetCustomer(ctx, customerID), questions.ForgetCustomer(ctx, customerID))
	}
	if subscriber != nil {
		if err := subscriber.OnUserDeleted(workerCtx, forget); err != nil {
			cancelWorkers()
			_ = closePublisher()
			_ = store.close()
			return nil, fmt.Errorf("subscribe user.deleted: %w", err)
		}
	}

	// The inbox is the only authenticated surface here, and the only place a
	// shopper's conversation can be read in full. Without a validator those
	// routes answer 503 rather than serving unauthenticated.
	var jwtValidator authjwt.AccessTokenValidator
	if cfg.JWKSURL != "" || cfg.JWTSecret != "" {
		jwtValidator, err = authjwt.NewAccessTokenValidator(cfg.JWKSURL, cfg.JWTSecret)
		if err != nil {
			cancelWorkers()
			_ = closePublisher()
			_ = store.close()
			return nil, fmt.Errorf("auth validator: %w", err)
		}
	} else {
		log.Println("WARNING: neither AUTH_JWKS_URL nor JWT_SECRET is set — the manager inbox will answer 503")
	}

	h := handler.New(handler.Options{
		Updates:       processor,
		Inbox:         inbox,
		Answers:       store.answers,
		JWTValidator:  jwtValidator,
		WebhookSecret: cfg.TelegramWebhookSecret,
		Settings:      BuildSettings(cfg),
		UpdateContext: workerCtx,
		WebChat:       webChat,
		Hub:           hub,
		Questions:     questions,
	})

	// Close what nobody has touched, so the queue shows live work rather than
	// history. Conservative on purpose: a consultation still being answered is
	// never stale, however long it runs.
	go runStaleCloser(workerCtx, inbox, cfg.InquiryQuietPeriod)

	// Retention is a promise to shoppers: transcripts hold whatever they typed,
	// so the words go on schedule even if nobody remembers to ask.
	retention := cfg.MessageRetention
	if retention == 0 {
		retention = DefaultMessageRetention
	}
	go runRetentionPurge(workerCtx, inbox, retention)

	// The shopper hears about a reply they have not read, by email.
	go runReplyNotices(workerCtx, webChat, cfg.ReplyNoticeDelay)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	httpSrv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}
	// Ends open SSE streams (they watch workerCtx) so a graceful shutdown is
	// not held open by them.
	httpSrv.RegisterOnShutdown(cancelWorkers)

	if client.Enabled() {
		if err := startInbound(workerCtx, client, processor, cfg); err != nil {
			cancelWorkers()
			_ = closePublisher()
			_ = store.close()
			return nil, err
		}
	} else {
		log.Println("TELEGRAM_SUPPORT_BOT_TOKEN not set — the support bot is inert (HTTP still serves)")
	}

	return &App{
		Router:        mux,
		HTTP:          httpSrv,
		cancelWorkers: cancelWorkers,
		close: func() error {
			return errors.Join(closePublisher(), store.close())
		},
	}, nil
}

// newBotClient builds the Bot API client, honouring a mock API base when one
// is configured.
//
// A non-default base is logged loudly rather than silently obeyed: it means
// this process is not talking to Telegram, which is correct in a smoke test and
// a serious incident in production.
func newBotClient(cfg Config) *tg.Client {
	if base := strings.TrimSpace(cfg.TelegramAPIBase); base != "" {
		log.Printf("WARNING: support bot is using a non-default Telegram API base (%s) — local/dev only", base)
		return tg.NewClientWithAPIBase(cfg.TelegramToken, nil, base)
	}
	return tg.NewClient(cfg.TelegramToken, nil)
}

// startInbound registers the webhook, or falls back to long-polling when no
// webhook URL is configured — which is how local dev runs without a public URL.
func startInbound(ctx context.Context, client *tg.Client, processor tg.Handler, cfg Config) error {
	if cfg.TelegramWebhookURL == "" {
		go tg.RunPoller(ctx, client, processor)
		return nil
	}

	// Telegram answers getUpdates with 409 while a webhook is active, so the
	// backlog is drained with no webhook registered — including one left over
	// from a previous run.
	if err := client.DeleteWebhook(ctx); err != nil {
		log.Printf("support telegram deleteWebhook before drain: %v", err)
	}
	if err := tg.DrainUpdates(ctx, client, processor); err != nil {
		log.Printf("support telegram drain updates: %v", err)
	}
	// Menus are the whole point of this bot, and Telegram drops any update type
	// not named here — an unlisted button tap never arrives and the button
	// spins on the shopper's device forever.
	err := client.SetWebhook(ctx, cfg.TelegramWebhookURL, cfg.TelegramWebhookSecret,
		tg.WithAllowedUpdates("message", "callback_query"))
	if err != nil {
		return fmt.Errorf("set telegram webhook: %w", err)
	}
	log.Printf("support telegram webhook registered at %s", cfg.TelegramWebhookURL)
	return nil
}

// store is the set of repositories the router needs, and how to let them go.
type store struct {
	conversations ports.ConversationRepository
	answers       ports.AnswerRepository
	inquiries     ports.InquiryRepository
	messages      ports.MessageRepository
	questions     ports.ProductQuestionRepository
	close         func() error
}

// openStore picks storage: PostgreSQL when DUPLI1_SUPPORT_DB is set, otherwise
// in-memory, as order, cart and payment already do. Tests and a local start
// need no database.
//
// Seeding happens here, at boot, and only fills nodes that have no row — staff
// edit this copy from the manager inbox, and a deploy that overwrote their
// wording every restart would make that editor pointless.
func openStore(connString string) (*store, error) {
	if strings.TrimSpace(connString) == "" {
		log.Println("DUPLI1_SUPPORT_DB not set — conversations are in memory and will not survive a restart")
		return &store{
			conversations: memory.NewConversationRepository(),
			answers:       memory.NewAnswerRepository(),
			inquiries:     memory.NewInquiryRepository(),
			messages:      memory.NewMessageRepository(),
			questions:     memory.NewProductQuestionRepository(),
			close:         func() error { return nil },
		}, nil
	}

	db, err := postgres.Open(connString)
	if err != nil {
		return nil, err
	}
	seedCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	seeded, err := postgres.SeedAnswers(seedCtx, db, domain.DefaultLanguage)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if seeded > 0 {
		log.Printf("seeded %d support answer(s)", seeded)
	}
	return &store{
		conversations: postgres.NewConversationRepository(db),
		answers:       postgres.NewAnswerRepository(db),
		inquiries:     postgres.NewInquiryRepository(db),
		messages:      postgres.NewMessageRepository(db),
		questions:     postgres.NewProductQuestionRepository(db),
		close:         db.Close,
	}, nil
}

// openPublisher connects the escalation announcement to NATS.
//
// Without NATS the bot still consults and still records inquiries; it simply
// cannot tell staff, which is logged loudly rather than passed over — an
// inquiry nobody hears about is the failure this whole phase exists to prevent.
func openPublisher(cfg Config) (ports.InquiryPublisher, func() error, error) {
	if strings.TrimSpace(cfg.NATSURL) == "" {
		log.Println("WARNING: NATS_URL not set — escalations are recorded but no ops alert is sent")
		return nil, func() error { return nil }, nil
	}
	publisher, err := natspublisher.New(cfg.NATSURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect nats: %w", err)
	}
	return natsinfra.NewInquiryPublisher(publisher, cfg.ManageWebURL), func() error {
		publisher.Close()
		return nil
	}, nil
}

// openLive picks how live changes travel. With NATS: publish to the bus and
// relay the bus (every replica's events) into this replica's hub. Without it:
// straight into the hub, which is right for a single process.
func openLive(cfg Config, hub *livefeed.Hub) (ports.LivePublisher, *natsinfra.Subscriber, func() error, error) {
	if strings.TrimSpace(cfg.NATSURL) == "" {
		return hub, nil, func() error { return nil }, nil
	}
	publisher, err := natspublisher.New(cfg.NATSURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect nats (live): %w", err)
	}
	subscriber, err := natsinfra.NewSubscriber(cfg.NATSURL)
	if err != nil {
		publisher.Close()
		return nil, nil, nil, err
	}
	if err := subscriber.RelayLive(hub); err != nil {
		subscriber.Close()
		publisher.Close()
		return nil, nil, nil, fmt.Errorf("subscribe live events: %w", err)
	}
	return natsinfra.NewLivePublisher(publisher), subscriber, func() error {
		subscriber.Close()
		publisher.Close()
		return nil
	}, nil
}

// openReferenceReaders reads products and orders through the internal
// gateway. Without one, interfaces stay nil (not typed-nil pointers) so the
// chat refuses references with 503 instead of panicking.
func openReferenceReaders(cfg Config) (ports.ProductReader, ports.OrderReader) {
	base := strings.TrimRight(strings.TrimSpace(cfg.GatewayURL), "/")
	if base == "" {
		log.Println("WARNING: DUPLI1_GATEWAY_URL not set — web chat cannot attach products or orders")
		return nil, nil
	}
	return gatewayinfra.NewProductReader(base, nil), gatewayinfra.NewOrderReader(base, nil)
}

// openNotifier builds the reply-notice sender. Unset SMTP or storefront URL
// disables notices (logged); a set but unreadable SMTP config fails the boot,
// since a typo there would otherwise silently drop every notice.
func openNotifier(cfg Config) (ports.ShopperNotifier, string, error) {
	storefront := strings.TrimRight(strings.TrimSpace(cfg.StorefrontURL), "/")
	if strings.TrimSpace(cfg.SMTP.Addr) == "" || storefront == "" {
		log.Println("WARNING: DUPLI1_SUPPORT_SMTP_ADDR or DUPLI1_STOREFRONT_URL not set — shoppers get no email about unread replies")
		return nil, "", nil
	}
	notifier, err := emailinfra.NewSMTPNotifier(emailinfra.SMTPConfig{
		Addr:     cfg.SMTP.Addr,
		Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
	})
	if err != nil {
		return nil, "", fmt.Errorf("reply notice: %w", err)
	}
	return notifier, storefront + "/profile/support", nil
}

func joinClose(fns ...func() error) func() error {
	return func() error {
		var errs []error
		for _, fn := range fns {
			errs = append(errs, fn())
		}
		return errors.Join(errs...)
	}
}

// runReplyNotices emails shoppers about replies left unread. Every minute, so
// a notice goes out between delay and delay+1m after the reply.
func runReplyNotices(ctx context.Context, webChat *service.WebChat, delay time.Duration) {
	if delay <= 0 {
		delay = DefaultReplyNoticeDelay
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sent, err := webChat.SendDueNotices(ctx, delay)
			if err != nil {
				log.Printf("support reply notices: %v", err)
			}
			if sent > 0 {
				log.Printf("sent %d reply notice(s)", sent)
			}
		}
	}
}

// runStaleCloser sweeps abandoned inquiries closed.
func runStaleCloser(ctx context.Context, inbox *service.Inbox, quietFor time.Duration) {
	if quietFor <= 0 {
		quietFor = DefaultInquiryQuietPeriod
	}
	// Hourly is plenty for a seven-day window and costs one indexed query.
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			closed, err := inbox.CloseStale(ctx, quietFor)
			if err != nil {
				log.Printf("close stale inquiries: %v", err)
				continue
			}
			if closed > 0 {
				log.Printf("closed %d inquiry(ies) after %s of silence", closed, quietFor)
			}
		}
	}
}

// runRetentionPurge drops message text past its retention window.
//
// It sweeps once at start and then daily: a deploy should not be able to
// postpone a purge that was already due, which an interval-only ticker would
// do on a service that restarts often.
func runRetentionPurge(ctx context.Context, inbox *service.Inbox, retention time.Duration) {
	if retention <= 0 {
		log.Println("message retention is disabled — transcripts are kept indefinitely")
		return
	}

	purge := func() {
		purged, err := inbox.PurgeExpiredBodies(ctx, retention)
		if err != nil {
			log.Printf("purge expired message bodies: %v", err)
			return
		}
		if purged > 0 {
			log.Printf("purged %d message body(ies) older than %s", purged, retention)
		}
	}
	purge()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purge()
		}
	}
}

var ulidEntropy = ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)

func newULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy).String()
}
