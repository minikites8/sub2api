//go:build integration

package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

func TestGPTAutoPhase21UpgradeDefaultsOffPreservesHistory(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, selectDockerImage(ctx, postgresImageTag), tcpostgres.WithDatabase("phase21"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)
	db, err := openSQLWithRetry(ctx, dsn, 30*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	previous := fstest.MapFS{}
	names, err := fs.Glob(migrations.FS, "*.sql")
	require.NoError(t, err)
	for _, name := range names {
		if name == "282_gpt_auto_independent_pricing.sql" {
			continue
		}
		raw, err := fs.ReadFile(migrations.FS, name)
		require.NoError(t, err)
		previous[name] = &fstest.MapFile{Data: raw}
	}
	require.NoError(t, applyMigrationsFS(ctx, db, previous))
	_, err = db.ExecContext(ctx, `INSERT INTO groups(name,platform,model_allowlist) VALUES ('phase2-existing','openai','{"enabled":true,"models":["gpt-auto","gpt-6-luna"]}');
 INSERT INTO gpt_auto_shadow_decisions(request_id,mode,evaluation_status,actual_model,final_model,policy_version,est_revenue) VALUES ('phase2-history','live','success','gpt-auto','gpt-6-luna','auto-v2',0.0123456789)`)
	require.NoError(t, err)
	var before string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT model_allowlist::text FROM groups WHERE name='phase2-existing'`).Scan(&before))
	require.NoError(t, applyMigrationsFS(ctx, db, migrations.FS))
	require.NoError(t, applyMigrationsFS(ctx, db, migrations.FS))
	var enabled bool
	var after string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT gpt_auto_enabled,model_allowlist::text FROM groups WHERE name='phase2-existing'`).Scan(&enabled, &after))
	require.False(t, enabled)
	require.Equal(t, before, after)
	var price, mode string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT est_revenue::text,pricing_mode FROM gpt_auto_shadow_decisions WHERE request_id='phase2-history'`).Scan(&price, &mode))
	require.Equal(t, "0.0123456789", price)
	require.Empty(t, mode)
	var on int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM groups WHERE gpt_auto_enabled`).Scan(&on))
	require.Zero(t, on)
}

func TestGPTAutoPricingCASAcrossConcurrentWriters(t *testing.T) {
	repo := NewSettingRepository(integrationEntClient).(*settingRepository)
	key := "gpt_auto_cas_test_" + time.Now().Format("150405.000000")
	require.NoError(t, repo.Set(context.Background(), key, "previous"))
	var successes atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.CompareAndSwap(context.Background(), key, "previous", "next")
			if err == nil && ok {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1), successes.Load())
}

func TestGPTAutoGroupAuthProjection(t *testing.T) {
	ctx := context.Background()
	g, err := integrationEntClient.Group.Create().SetName("phase21-auth-" + time.Now().Format("150405.000000")).SetPlatform(service.PlatformOpenAI).SetGptAutoEnabled(true).Save(ctx)
	require.NoError(t, err)
	user := mustCreateAPIKeyRepoUser(t, ctx, integrationEntClient, "phase21-"+time.Now().Format("150405.000000")+"@example.test")
	key := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: user.ID, GroupID: &g.ID, Key: "sk-phase21-" + time.Now().Format("150405.000000"), Name: "phase21", Status: service.StatusActive})
	repo := NewAPIKeyRepository(integrationEntClient, integrationDB)
	loaded, err := repo.GetByKeyForAuth(ctx, key.Key)
	require.NoError(t, err)
	require.True(t, loaded.Group.GPTAutoEnabled)
}
