package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestGPTAutoDynamicPerTurn(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.DynamicRouting = DefaultGPTAutoDynamicConfig()
		c.Live.DynamicRouting.Enabled = true
	})
	input := gptAutoLiveInput(gptAutoLiveChatBody)
	input.SessionKey = "same-agent"
	cases := []struct{ model, effort, want, reason string }{
		{GPTAutoModelGPT6Luna, "low", GPTAutoModelGPT6Luna, ""},
		{GPTAutoModelGPT61Sol, "high", GPTAutoModelGPT61Sol, ""},
		{GPTAutoModelGPT61Sol, "medium", GPTAutoModelGPT61Sol, ""},
		{GPTAutoModelGPT6Luna, "low", GPTAutoModelGPT61Sol, "downgrade_hysteresis"},
		{GPTAutoModelGPT6Luna, "low", GPTAutoModelGPT6Luna, ""},
		{GPTAutoModelGPT56Luna, "none", GPTAutoModelGPT6Luna, "downgrade_hysteresis"},
		{GPTAutoModelGPT56Luna, "none", GPTAutoModelGPT56Luna, ""},
	}
	for i, tc := range cases {
		gptAutoClassifyAs(classifier, tc.model, tc.effort)
		r, err := svc.Route(context.Background(), input)
		require.NoError(t, err)
		require.Equal(t, tc.want, r.Model())
		require.Equal(t, tc.effort, r.Effort())
		require.Equal(t, tc.reason, r.row.Features.Switching.SwitchBlockReason)
		require.Equal(t, int64(i+1), r.row.Features.Switching.Sequence)
		require.NotNil(t, r.row.Classification)
		require.Equal(t, tc.model, r.row.JevModel)
		r.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200, AccountID: 1})
	}
}
func TestGPTAutoDynamicHardState(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.DynamicRouting = DefaultGPTAutoDynamicConfig()
		c.Live.DynamicRouting.Enabled = true
	})
	gptAutoClassifyAs(classifier, GPTAutoModelGPT6Luna, "low")
	in := gptAutoLiveInput(`{"model":"gpt-auto","input":"Build an app"}`)
	in.Endpoint = GPTAutoEndpointResponses
	r, err := svc.Route(context.Background(), in)
	require.NoError(t, err)
	r.ObserveResult(&OpenAIForwardResult{ResponseID: "resp-test"})
	r.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200, AccountID: 55})
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, "high")
	in.Body = []byte(`{"model":"gpt-auto","previous_response_id":"resp-test","input":"continue"}`)
	r, err = svc.Route(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT6Luna, r.Model())
	require.Nil(t, r.row.Classification)
	require.Nil(t, r.ValidateAccount(55))
	require.Equal(t, 409, r.ValidateAccount(56).Status)
	require.False(t, r.Reroute("no_upstream_available"))
	in.ModelPermitted = func(m string) bool { return m != GPTAutoModelGPT6Luna }
	_, err = svc.Route(context.Background(), in)
	require.Equal(t, 409, gptAutoRouteErr(t, err).Status)
	in.ModelPermitted = nil
	in.Body = []byte(`{"model":"gpt-auto","previous_response_id":"missing","input":"continue"}`)
	in.SessionKey = "some-soft-session"
	_, err = svc.Route(context.Background(), in)
	require.Equal(t, 409, gptAutoRouteErr(t, err).Status)
	in.Body = []byte(`{"model":"gpt-auto","conversation":"conv-unverified","input":"continue"}`)
	_, err = svc.Route(context.Background(), in)
	require.Equal(t, 409, gptAutoRouteErr(t, err).Status)
}
func TestGPTAutoDynamicNewTaskAndConcurrentTurns(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.DynamicRouting = DefaultGPTAutoDynamicConfig()
		c.Live.DynamicRouting.Enabled = true
	})
	in := gptAutoLiveInput(gptAutoLiveChatBody)
	in.SessionKey = "branch"
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, "high")
	r, err := svc.Route(context.Background(), in)
	require.NoError(t, err)
	r.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200, AccountID: 1})
	gptAutoClassifyAs(classifier, GPTAutoModelGPT6Luna, "low")
	a, err := svc.Route(context.Background(), in)
	require.NoError(t, err)
	b, err := svc.Route(context.Background(), in)
	require.NoError(t, err)
	a.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200, AccountID: 1})
	b.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200, AccountID: 1})
	entry, err := svc.live.affinity.Get(context.Background(), gptAutoAffinityKey(22, "sess", "branch"))
	require.NoError(t, err)
	require.Equal(t, 1, entry.DowngradeCount)
	require.Equal(t, int64(2), entry.Sequence)
	in.Body = []byte(`{"model":"gpt-auto","messages":[{"role":"user","content":"Write unrelated documentation"}]}`)
	r, err = svc.Route(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT61Sol, r.Model())
	require.Equal(t, 1, r.downgradeCount)
}
func TestGPTAutoDynamicConfigValidation(t *testing.T) {
	c := DefaultGPTAutoDynamicConfig()
	require.False(t, c.Enabled)
	require.NoError(t, validateGPTAutoDynamic(c))
	c.EnableClassificationCache = true
	require.Error(t, validateGPTAutoDynamic(c))
	c.EnableClassificationCache = false
	c.PreserveAccountAffinity = false
	require.Error(t, validateGPTAutoDynamic(c))
	c.PreserveAccountAffinity = true
	c.CapabilityTiers[GPTAutoModelGPT6Luna] = 0
	require.Error(t, validateGPTAutoDynamic(c))
}
func BenchmarkGPTAutoDynamicSessionCAS(b *testing.B) {
	store := newLocalGPTAutoAffinityStore()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.CompareAndSet(ctx, "session", int64(i), GPTAutoAffinity{Sequence: int64(i + 1)}, time.Hour)
	}
}

func TestGPTAutoDynamicIndependentFlagsAndEffortCap(t *testing.T) {
	for _, tc := range []struct {
		name, old, next            string
		upgrade, downgrade, effort bool
		want                       string
	}{
		{"upgrade disabled", GPTAutoModelGPT6Luna, GPTAutoModelGPT61Sol, false, true, true, GPTAutoModelGPT6Luna},
		{"downgrade disabled", GPTAutoModelGPT61Sol, GPTAutoModelGPT6Luna, true, false, true, GPTAutoModelGPT61Sol},
		{"effort independent", GPTAutoModelGPT6Luna, GPTAutoModelGPT6Luna, true, true, true, GPTAutoModelGPT6Luna},
		{"effort locked", GPTAutoModelGPT6Luna, GPTAutoModelGPT6Luna, true, true, false, GPTAutoModelGPT6Luna},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
				c.Live.DynamicRouting = DefaultGPTAutoDynamicConfig()
				d := c.Live.DynamicRouting
				d.Enabled = true
				d.AllowModelUpgrade = tc.upgrade
				d.AllowModelDowngrade = tc.downgrade
				d.AllowEffortChange = tc.effort
			})
			in := gptAutoLiveInput(gptAutoLiveChatBody)
			in.SessionKey = "flags"
			gptAutoClassifyAs(classifier, tc.old, "low")
			r, err := svc.Route(context.Background(), in)
			require.NoError(t, err)
			r.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200, AccountID: 1})
			gptAutoClassifyAs(classifier, tc.next, "high")
			r, err = svc.Route(context.Background(), in)
			require.NoError(t, err)
			require.Equal(t, tc.want, r.Model())
			if tc.effort {
				require.Equal(t, "high", r.Effort())
			} else {
				require.Equal(t, "low", r.Effort())
			}
			r.ApplyExecutionEffort([]byte(`{"reasoning_effort":"low"}`))
			require.Equal(t, "low", r.Effort())
			require.Equal(t, "low", r.row.FinalEffort)
		})
	}
}

type unavailableGPTAutoAffinity struct{}

func (unavailableGPTAutoAffinity) Get(context.Context, string) (*GPTAutoAffinity, error) {
	return nil, context.DeadlineExceeded
}
func (unavailableGPTAutoAffinity) Set(context.Context, string, GPTAutoAffinity, time.Duration) error {
	return context.DeadlineExceeded
}
func TestGPTAutoDynamicAffinityOutage(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.DynamicRouting = DefaultGPTAutoDynamicConfig()
		c.Live.DynamicRouting.Enabled = true
	})
	svc.SetAffinityStore(unavailableGPTAutoAffinity{})
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, "high")
	in := gptAutoLiveInput(gptAutoLiveChatBody)
	in.SessionKey = "missing-redis"
	r, err := svc.Route(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, GPTAutoAffinityUnavailable, r.row.AffinityOutcome)
	require.NotNil(t, r.row.Classification)
	in.Endpoint = GPTAutoEndpointResponses
	in.Body = []byte(`{"model":"gpt-auto","previous_response_id":"unknown","input":"continue"}`)
	_, err = svc.Route(context.Background(), in)
	require.Equal(t, 409, gptAutoRouteErr(t, err).Status)
}

func TestGPTAutoDynamicCapacityFallbackRespectsSwitchControls(t *testing.T) {
	svc, _, classifier := newGPTAutoLiveTestService(t, func(c *GPTAutoShadowConfig) {
		c.Live.DynamicRouting = DefaultGPTAutoDynamicConfig()
		c.Live.DynamicRouting.Enabled = true
		c.Live.DynamicRouting.AllowModelUpgrade = false
		c.Live.DynamicRouting.AllowModelDowngrade = false
	})
	in := gptAutoLiveInput(gptAutoLiveChatBody)
	in.SessionKey = "capacity"
	gptAutoClassifyAs(classifier, GPTAutoModelGPT6Luna, "low")
	r, err := svc.Route(context.Background(), in)
	require.NoError(t, err)
	r.Finish(context.Background(), GPTAutoRouteOutcome{HTTPStatus: 200, AccountID: 1})
	gptAutoClassifyAs(classifier, GPTAutoModelGPT61Sol, "high")
	r, err = svc.Route(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, GPTAutoModelGPT6Luna, r.Model())
	require.False(t, r.Reroute(GPTAutoFailureNoUpstream))
}
