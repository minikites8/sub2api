package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

func initializeNodeApplication(cfg *config.Config, buildInfo handler.BuildInfo) (*Application, error) {
	if cfg == nil {
		loaded, err := config.LoadForBootstrap()
		if err != nil {
			return nil, err
		}
		cfg = loaded
	}

	entClient, err := repository.ProvideEnt(cfg)
	if err != nil {
		return nil, err
	}
	sqlDB, err := repository.ProvideSQLDB(entClient)
	if err != nil {
		_ = entClient.Close()
		return nil, err
	}
	redisClient := repository.ProvideRedis(cfg)

	userRepo := repository.NewUserRepository(entClient, sqlDB)
	groupRepo := repository.NewGroupRepository(entClient, sqlDB)
	proxyRepo := repository.NewProxyRepository(entClient, sqlDB)
	usageBillingRepo := repository.NewUsageBillingRepository(entClient, sqlDB)
	quotaLeasePersistenceStore := repository.NewQuotaLeaseDemoPersistenceStore(sqlDB)
	settingRepo := repository.NewSettingRepository(entClient)
	settingService := service.ProvideSettingService(settingRepo, groupRepo, proxyRepo, cfg, usageBillingRepo, quotaLeasePersistenceStore)

	userSubRepo := repository.NewUserSubscriptionRepository(entClient)
	apiKeyRepo := repository.NewAPIKeyRepository(entClient, sqlDB)
	userRPMCache := repository.NewUserRPMCache(redisClient)
	userGroupRateRepo := repository.NewUserGroupRateRepository(sqlDB)
	userPlatformQuotaRepo := repository.NewUserPlatformQuotaRepository(entClient)
	userPlatformQuotaAdapter := repository.NewUserPlatformQuotaServiceAdapter(userPlatformQuotaRepo)
	billingCache := repository.NewBillingCache(redisClient)
	apiKeyCache := repository.NewAPIKeyCache(redisClient)
	concurrencyCache := repository.ProvideConcurrencyCache(redisClient, cfg)
	schedulerCache := repository.ProvideSchedulerCache(redisClient, cfg)
	accountRepo := repository.NewAccountRepository(entClient, sqlDB, schedulerCache)

	concurrencyService := service.ProvideConcurrencyService(concurrencyCache, accountRepo, cfg)
	billingCacheService := service.ProvideBillingCacheService(
		billingCache,
		userRepo,
		userSubRepo,
		apiKeyRepo,
		userRPMCache,
		userGroupRateRepo,
		cfg,
		userPlatformQuotaAdapter,
	)
	apiKeyService := service.ProvideAPIKeyService(
		apiKeyRepo,
		userRepo,
		groupRepo,
		userSubRepo,
		userGroupRateRepo,
		apiKeyCache,
		cfg,
		billingCacheService,
		concurrencyService,
	)
	apiKeyAuthCacheInvalidator := service.ProvideAPIKeyAuthCacheInvalidator(apiKeyService)
	subscriptionService := service.NewSubscriptionService(groupRepo, userSubRepo, billingCacheService, entClient, cfg)
	userService := service.NewUserService(userRepo, settingRepo, apiKeyAuthCacheInvalidator, billingCache)
	usageLogRepo := repository.NewUsageLogRepository(entClient, sqlDB)
	usageService := service.NewUsageService(usageLogRepo, userRepo, entClient, apiKeyAuthCacheInvalidator)
	gatewayCache := repository.NewGatewayCache(redisClient)
	schedulerOutboxRepo := repository.NewSchedulerOutboxRepository(sqlDB)
	schedulerSnapshot := service.ProvideSchedulerSnapshotService(schedulerCache, schedulerOutboxRepo, accountRepo, groupRepo, cfg)
	pricingRemoteClient := repository.ProvidePricingRemoteClient(cfg)
	pricingService, err := service.ProvidePricingService(cfg, pricingRemoteClient)
	if err != nil {
		cleanupNodeInfra(entClient, redisClient)
		return nil, err
	}
	billingService := service.NewBillingService(cfg, pricingService)
	geminiQuotaService := service.NewGeminiQuotaService(cfg, settingRepo)
	tempUnschedCache := repository.NewTempUnschedCache(redisClient)
	timeoutCounterCache := repository.NewTimeoutCounterCache(redisClient)
	openAI403CounterCache := repository.NewOpenAI403CounterCache(redisClient)
	internal500CounterCache := repository.NewInternal500CounterCache(redisClient)
	geminiTokenCache := repository.NewGeminiTokenCache(redisClient)
	compositeTokenCacheInvalidator := service.NewCompositeTokenCacheInvalidator(geminiTokenCache)
	rateLimitService := service.ProvideRateLimitService(
		accountRepo,
		usageLogRepo,
		cfg,
		geminiQuotaService,
		tempUnschedCache,
		timeoutCounterCache,
		openAI403CounterCache,
		settingService,
		compositeTokenCacheInvalidator,
	)
	identityCache := repository.NewIdentityCache(redisClient)
	identityService := service.NewIdentityService(identityCache)
	httpUpstream := repository.NewHTTPUpstream(cfg)
	timingWheel, err := service.ProvideTimingWheelService()
	if err != nil {
		cleanupNodeInfra(entClient, redisClient)
		return nil, err
	}
	deferredService := service.ProvideDeferredService(accountRepo, timingWheel)

	claudeOAuthClient := repository.NewClaudeOAuthClient()
	oauthService := service.NewOAuthService(proxyRepo, claudeOAuthClient)
	oauthRefreshAPI := service.ProvideOAuthRefreshAPI(accountRepo, geminiTokenCache)
	claudeTokenProvider := service.ProvideClaudeTokenProvider(accountRepo, geminiTokenCache, oauthService, oauthRefreshAPI)
	openAIOAuthClient := repository.NewOpenAIOAuthClient()
	openAIOAuthService := service.ProvideOpenAIOAuthService(proxyRepo, openAIOAuthClient, providePrivacyClientFactory())
	openAITokenProvider := service.ProvideOpenAITokenProvider(accountRepo, geminiTokenCache, openAIOAuthService, oauthRefreshAPI)
	grokOAuthClient := repository.NewGrokOAuthClient()
	grokOAuthService := service.NewGrokOAuthService(proxyRepo, grokOAuthClient)
	grokTokenProvider := service.ProvideGrokTokenProvider(accountRepo, geminiTokenCache, grokOAuthService, oauthRefreshAPI, tempUnschedCache)
	geminiOAuthClient := repository.NewGeminiOAuthClient(cfg)
	geminiCliCodeAssistClient := repository.NewGeminiCliCodeAssistClient()
	driveClient := repository.NewGeminiDriveClient()
	geminiOAuthService := service.NewGeminiOAuthService(proxyRepo, geminiOAuthClient, geminiCliCodeAssistClient, driveClient, cfg)
	geminiTokenProvider := service.ProvideGeminiTokenProvider(accountRepo, geminiTokenCache, geminiOAuthService, oauthRefreshAPI)
	antigravityOAuthService := service.NewAntigravityOAuthService(proxyRepo)
	antigravityTokenProvider := service.ProvideAntigravityTokenProvider(accountRepo, geminiTokenCache, antigravityOAuthService, oauthRefreshAPI, tempUnschedCache)
	kiroOAuthService := service.NewKiroOAuthService(proxyRepo)
	kiroTokenProvider := service.ProvideKiroTokenProvider(accountRepo, geminiTokenCache, kiroOAuthService, oauthRefreshAPI)
	kiroCooldownStore := service.ProvideKiroCooldownStore(redisClient)
	sessionLimitCache := repository.ProvideSessionLimitCache(redisClient, cfg)
	rpmCache := repository.NewRPMCache(redisClient)
	digestSessionStore := service.NewDigestSessionStore()
	tlsFPRepo := repository.NewTLSFingerprintProfileRepository(entClient)
	tlsFPCache := repository.NewTLSFingerprintProfileCache(redisClient)
	tlsFPService := service.NewTLSFingerprintProfileService(tlsFPRepo, tlsFPCache)
	channelRepo := repository.NewChannelRepository(sqlDB)
	channelService := service.NewChannelService(channelRepo, groupRepo, apiKeyAuthCacheInvalidator, pricingService)
	modelPricingResolver := service.NewModelPricingResolver(channelService, billingService)

	gatewayService := service.NewGatewayService(
		accountRepo,
		groupRepo,
		usageLogRepo,
		usageBillingRepo,
		userRepo,
		userSubRepo,
		userGroupRateRepo,
		gatewayCache,
		cfg,
		schedulerSnapshot,
		concurrencyService,
		billingService,
		rateLimitService,
		billingCacheService,
		identityService,
		httpUpstream,
		deferredService,
		claudeTokenProvider,
		kiroTokenProvider,
		kiroCooldownStore,
		sessionLimitCache,
		rpmCache,
		digestSessionStore,
		settingService,
		apiKeyAuthCacheInvalidator,
		tlsFPService,
		channelService,
		modelPricingResolver,
		nil,
		userPlatformQuotaAdapter,
	)
	openAIGatewayService := service.NewOpenAIGatewayService(
		accountRepo,
		usageLogRepo,
		usageBillingRepo,
		userRepo,
		userSubRepo,
		userGroupRateRepo,
		gatewayCache,
		cfg,
		schedulerSnapshot,
		concurrencyService,
		billingService,
		rateLimitService,
		billingCacheService,
		httpUpstream,
		deferredService,
		openAITokenProvider,
		grokTokenProvider,
		modelPricingResolver,
		channelService,
		nil,
		settingService,
		apiKeyAuthCacheInvalidator,
		userPlatformQuotaAdapter,
	)
	quotaMirrorStore := repository.NewQuotaLeaseDemoMirrorStore(entClient, sqlDB, accountRepo, apiKeyAuthCacheInvalidator, openAIGatewayService)
	tokenRefreshService := service.ProvideTokenRefreshService(
		accountRepo,
		oauthService,
		openAIOAuthService,
		geminiOAuthService,
		antigravityOAuthService,
		kiroOAuthService,
		grokOAuthService,
		compositeTokenCacheInvalidator,
		schedulerCache,
		cfg,
		tempUnschedCache,
		providePrivacyClientFactory(),
		proxyRepo,
		oauthRefreshAPI,
		openAIGatewayService,
	)
	antigravityGatewayService := service.NewAntigravityGatewayService(
		accountRepo,
		gatewayCache,
		schedulerSnapshot,
		antigravityTokenProvider,
		rateLimitService,
		httpUpstream,
		settingService,
		internal500CounterCache,
	)
	geminiCompatService := service.NewGeminiMessagesCompatService(
		accountRepo,
		groupRepo,
		gatewayCache,
		schedulerSnapshot,
		geminiTokenProvider,
		rateLimitService,
		httpUpstream,
		antigravityGatewayService,
		cfg,
	)
	claudeUsageFetcher := repository.NewClaudeUsageFetcher(httpUpstream)
	antigravityQuotaFetcher := service.NewAntigravityQuotaFetcher(proxyRepo)
	grokQuotaFetcher := service.NewGrokQuotaFetcher()
	grokQuotaService := service.ProvideGrokQuotaService(accountRepo, proxyRepo, grokTokenProvider, httpUpstream, cfg, usageLogRepo)
	openAIQuotaService := service.ProvideOpenAIQuotaService(accountRepo, proxyRepo, openAITokenProvider, providePrivacyClientFactory(), openAIGatewayService)
	usageCache := service.NewUsageCache()
	accountUsageService := service.ProvideAccountUsageService(
		accountRepo,
		usageLogRepo,
		claudeUsageFetcher,
		geminiQuotaService,
		antigravityQuotaFetcher,
		grokQuotaFetcher,
		grokQuotaService,
		openAIQuotaService,
		usageCache,
		identityCache,
		tlsFPService,
		openAIGatewayService,
		kiroTokenProvider,
	)
	opsService := service.NewOpsService(
		nil,
		settingRepo,
		cfg,
		accountRepo,
		userRepo,
		concurrencyService,
		gatewayService,
		openAIGatewayService,
		geminiCompatService,
		antigravityGatewayService,
		nil,
	)
	quotaLeaseDemoNodeWorker := service.ProvideQuotaLeaseDemoNodeWorker(
		cfg,
		openAIOAuthService,
		grokOAuthService,
		quotaMirrorStore,
		accountUsageService,
		openAIQuotaService,
		grokQuotaService,
		channelService,
	)

	usageRecordWorkerPool := service.NewUsageRecordWorkerPool(cfg)
	userMsgQueueCache := repository.NewUserMsgQueueCache(redisClient)
	userMessageQueueService := service.ProvideUserMessageQueueService(userMsgQueueCache, rpmCache, cfg)
	gatewayHandler := handler.NewGatewayHandler(
		gatewayService,
		openAIGatewayService,
		geminiCompatService,
		antigravityGatewayService,
		userService,
		concurrencyService,
		billingCacheService,
		usageService,
		apiKeyService,
		usageRecordWorkerPool,
		nil,
		nil,
		userMessageQueueService,
		cfg,
		settingService,
	)
	openAIGatewayHandler := handler.NewOpenAIGatewayHandler(
		openAIGatewayService,
		concurrencyService,
		billingCacheService,
		apiKeyService,
		usageRecordWorkerPool,
		nil,
		nil,
		nil,
		cfg,
	)
	handlers := &handler.Handlers{
		Gateway:       gatewayHandler,
		OpenAIGateway: openAIGatewayHandler,
	}
	apiKeyAuth := middleware.NewAPIKeyAuthMiddleware(apiKeyService, subscriptionService, cfg)
	router := server.ProvideRouter(cfg, handlers, nil, nil, apiKeyAuth, apiKeyService, subscriptionService, opsService, settingService, redisClient)
	httpServer := server.ProvideHTTPServer(cfg, router)

	cleanup := provideNodeCleanup(
		entClient,
		redisClient,
		schedulerSnapshot,
		tokenRefreshService,
		quotaLeaseDemoNodeWorker,
		pricingService,
		billingCacheService,
		usageRecordWorkerPool,
		subscriptionService,
		oauthService,
		openAIOAuthService,
		geminiOAuthService,
		antigravityOAuthService,
		kiroOAuthService,
		grokOAuthService,
		openAIGatewayService,
		deferredService,
		timingWheel,
		userMessageQueueService,
	)
	return &Application{
		Server:  httpServer,
		Cleanup: cleanup,
	}, nil
}

func cleanupNodeInfra(entClient *ent.Client, rdb *redis.Client) {
	if rdb != nil {
		_ = rdb.Close()
	}
	if entClient != nil {
		_ = entClient.Close()
	}
}

func provideNodeCleanup(
	entClient *ent.Client,
	rdb *redis.Client,
	schedulerSnapshot *service.SchedulerSnapshotService,
	tokenRefresh *service.TokenRefreshService,
	quotaLeaseDemoNodeWorker *service.QuotaLeaseDemoNodeWorker,
	pricing *service.PricingService,
	billingCache *service.BillingCacheService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	subscriptionService *service.SubscriptionService,
	oauth *service.OAuthService,
	openaiOAuth *service.OpenAIOAuthService,
	geminiOAuth *service.GeminiOAuthService,
	antigravityOAuth *service.AntigravityOAuthService,
	kiroOAuth *service.KiroOAuthService,
	grokOAuth *service.GrokOAuthService,
	openAIGateway *service.OpenAIGatewayService,
	deferredService *service.DeferredService,
	timingWheel *service.TimingWheelService,
	userMessageQueueService *service.UserMessageQueueService,
) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		type cleanupStep struct {
			name string
			fn   func() error
		}

		parallelSteps := []cleanupStep{
			{"SchedulerSnapshotService", func() error {
				if schedulerSnapshot != nil {
					schedulerSnapshot.Stop()
				}
				return nil
			}},
			{"TokenRefreshService", func() error {
				if tokenRefresh != nil {
					tokenRefresh.Stop()
				}
				return nil
			}},
			{"QuotaLeaseDemoNodeWorker", func() error {
				if quotaLeaseDemoNodeWorker != nil {
					return quotaLeaseDemoNodeWorker.StopAndDrain(ctx)
				}
				return nil
			}},
			{"SubscriptionService", func() error {
				if subscriptionService != nil {
					subscriptionService.Stop()
				}
				return nil
			}},
			{"PricingService", func() error {
				if pricing != nil {
					pricing.Stop()
				}
				return nil
			}},
			{"BillingCacheService", func() error {
				if billingCache != nil {
					billingCache.Stop()
				}
				return nil
			}},
			{"UsageRecordWorkerPool", func() error {
				if usageRecordWorkerPool != nil {
					usageRecordWorkerPool.Stop()
				}
				return nil
			}},
			{"UserMessageQueueService", func() error {
				if userMessageQueueService != nil {
					userMessageQueueService.Stop()
				}
				return nil
			}},
			{"OAuthService", func() error {
				if oauth != nil {
					oauth.Stop()
				}
				return nil
			}},
			{"OpenAIOAuthService", func() error {
				if openaiOAuth != nil {
					openaiOAuth.Stop()
				}
				return nil
			}},
			{"GeminiOAuthService", func() error {
				if geminiOAuth != nil {
					geminiOAuth.Stop()
				}
				return nil
			}},
			{"AntigravityOAuthService", func() error {
				if antigravityOAuth != nil {
					antigravityOAuth.Stop()
				}
				return nil
			}},
			{"KiroOAuthService", func() error {
				if kiroOAuth != nil {
					kiroOAuth.Stop()
				}
				return nil
			}},
			{"GrokOAuthService", func() error {
				if grokOAuth != nil {
					grokOAuth.Stop()
				}
				return nil
			}},
			{"OpenAIWSPool", func() error {
				if openAIGateway != nil {
					openAIGateway.CloseOpenAIWSPool()
				}
				return nil
			}},
			{"DeferredService", func() error {
				if deferredService != nil {
					deferredService.Stop()
				}
				return nil
			}},
			{"TimingWheelService", func() error {
				if timingWheel != nil {
					timingWheel.Stop()
				}
				return nil
			}},
		}

		infraSteps := []cleanupStep{
			{"Redis", func() error {
				if rdb == nil {
					return nil
				}
				return rdb.Close()
			}},
			{"Ent", func() error {
				if entClient == nil {
					return nil
				}
				return entClient.Close()
			}},
		}

		runParallel := func(steps []cleanupStep) {
			var wg sync.WaitGroup
			for i := range steps {
				step := steps[i]
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := step.fn(); err != nil {
						log.Printf("[Node Cleanup] %s failed: %v", step.name, err)
						return
					}
					log.Printf("[Node Cleanup] %s succeeded", step.name)
				}()
			}
			wg.Wait()
		}

		runSequential := func(steps []cleanupStep) {
			for i := range steps {
				step := steps[i]
				if err := step.fn(); err != nil {
					log.Printf("[Node Cleanup] %s failed: %v", step.name, err)
					continue
				}
				log.Printf("[Node Cleanup] %s succeeded", step.name)
			}
		}

		runParallel(parallelSteps)
		runSequential(infraSteps)

		select {
		case <-ctx.Done():
			log.Printf("[Node Cleanup] Warning: cleanup timed out after 10 seconds")
		default:
			log.Printf("[Node Cleanup] All cleanup steps completed")
		}
	}
}
