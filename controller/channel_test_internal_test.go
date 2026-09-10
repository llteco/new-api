package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidateChannelProxy(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{name: "empty"},
		{name: "http", proxy: "http://proxy.example:8080"},
		{name: "https", proxy: "https://proxy.example:8443"},
		{name: "socks5", proxy: "socks5://proxy.example"},
		{name: "socks5h", proxy: "socks5h://proxy.example:1080/"},
		{name: "unsupported", proxy: "ftp://proxy.example", wantErr: true},
		{name: "path", proxy: "socks5://proxy.example:1080/path", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting, err := common.Marshal(dto.ChannelSettings{Proxy: test.proxy})
			require.NoError(t, err)
			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Setting: common.GetPointer(string(setting)),
			}

			err = validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "invalid channel proxy")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateChannelRequiresNewAPIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL *string
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "blank", baseURL: common.GetPointer("  "), wantErr: true},
		{name: "configured", baseURL: common.GetPointer("https://new-api.example")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{
				Type:    constant.ChannelTypeNewAPI,
				BaseURL: test.baseURL,
			}

			err := validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "New API channel base URL cannot be empty")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewAPIChannelRegistration(t *testing.T) {
	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeNewAPI)

	require.True(t, ok)
	assert.Equal(t, constant.APITypeNewAPI, apiType)
	assert.Equal(t, "New API", constant.GetChannelTypeName(constant.ChannelTypeNewAPI))
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeNewAPI)
	assert.Empty(t, constant.ChannelBaseURLs[constant.ChannelTypeNewAPI])
}

func TestResponsesCompactChannelSupport(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		apiType     int
		want        bool
	}{
		{name: "OpenAI", channelType: constant.ChannelTypeOpenAI, apiType: constant.APITypeOpenAI, want: true},
		{name: "Azure", channelType: constant.ChannelTypeAzure, apiType: constant.APITypeOpenAI, want: true},
		{name: "Codex", channelType: constant.ChannelTypeCodex, apiType: constant.APITypeCodex, want: true},
		{name: "Advanced Custom", channelType: constant.ChannelTypeAdvancedCustom, apiType: constant.APITypeAdvancedCustom, want: true},
		{name: "Sub2API", channelType: constant.ChannelTypeSub2API, apiType: constant.APITypeSub2API, want: true},
		{name: "New API", channelType: constant.ChannelTypeNewAPI, apiType: constant.APITypeNewAPI, want: true},
		{name: "Anthropic", channelType: constant.ChannelTypeAnthropic, apiType: constant.APITypeAnthropic, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, common.SupportsResponsesCompact(test.channelType, test.apiType))
		})
	}
}

func TestMultiprotocolGatewayEndpointTypes(t *testing.T) {
	want := []constant.EndpointType{
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
		constant.EndpointTypeOpenAIResponseCompact,
		constant.EndpointTypeAnthropic,
		constant.EndpointTypeGemini,
		constant.EndpointTypeOpenAIAlphaSearch,
	}

	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeNewAPI, "gpt-5"))
	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeSub2API, "gpt-5"))
}

func TestCopyChannelRejectsInvalidLegacyProxySettings(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		Proxy: "socks5://proxy.example/legacy-path",
	})
	require.NoError(t, err)
	setting := string(settingBytes)
	origin := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Name:    "legacy proxy channel",
		Key:     "test-key",
		Models:  "gpt-test",
		Group:   "default",
		Setting: &setting,
	}
	require.NoError(t, db.Create(origin).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy", nil)

	CopyChannel(ctx)

	assert.Contains(t, recorder.Body.String(), "invalid channel settings")
	var channelCount int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)
}

func TestDeleteChannelResetsProxyCacheWhenPreReadFails(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	service.ResetProxyClientCache()
	t.Cleanup(service.ResetProxyClientCache)

	proxyURL := "http://proxy.example:8080"
	beforeDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "999999"}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/999999", nil)

	DeleteChannel(ctx)

	assert.Contains(t, recorder.Body.String(), `"success":true`)
	afterDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)
	assert.NotSame(t, beforeDelete, afterDelete)
}

func TestDeleteChannelBatchReportsAndAuditsActualDeletedCount(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{Name: "existing", Key: "test-key"}
	require.NoError(t, db.Create(channel).Error)

	requestBody, err := common.Marshal(ChannelBatch{Ids: []int{channel.Id, 999999}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/batch", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	DeleteChannelBatch(ctx)

	var response struct {
		Success bool  `json:"success"`
		Data    int64 `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, int64(1), response.Data)

	var auditLog model.Log
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	var auditData struct {
		Operation struct {
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	require.NoError(t, common.UnmarshalJsonStr(auditLog.Other, &auditData))
	assert.Equal(t, float64(1), auditData.Operation.Params["count"])
}

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	requestRules := []billingexpr.RequestRuleTrace{{
		Cond:       `param("service_tier") == "fast"`,
		Multiplier: 2,
		Matched:    true,
	}}
	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier:  "base",
		RequestRules: requestRules,
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.Equal(t, requestRules, other["request_rules"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryOnlyUsesAutoDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 1)
	require.Equal(t, 2, selected[0].Id)
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestSelectChannelsForAutomaticTestAutoBanOnlyUsesEligibleChannels(t *testing.T) {
	autoBanEnabled := 1
	autoBanDisabled := 0
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled, AutoBan: &autoBanEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled, AutoBan: &autoBanDisabled},
		{Id: 3, Status: common.ChannelStatusAutoDisabled, AutoBan: &autoBanEnabled},
		{Id: 4, Status: common.ChannelStatusManuallyDisabled, AutoBan: &autoBanEnabled},
		{Id: 5, Status: common.ChannelStatusEnabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeAutoBanOnly)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 3, selected[1].Id)
}

func TestRunChannelTestWorkersHonorsConfiguredConcurrency(t *testing.T) {
	originalInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = originalInterval })

	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
		{Id: 4, Status: common.ChannelStatusEnabled},
	}
	started := make(chan struct{}, len(channels))
	release := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	progress := make([]int, 0, len(channels)+1)
	summaryResult := make(chan channelTestSummary, 1)

	go func() {
		summaryResult <- runChannelTestWorkers(
			context.Background(),
			channels,
			2,
			func(_ context.Context, _ *model.Channel) channelTestSummary {
				current := active.Add(1)
				defer active.Add(-1)
				for {
					observed := maxActive.Load()
					if current <= observed || maxActive.CompareAndSwap(observed, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				return channelTestSummary{Tested: 1, Succeeded: 1}
			},
			func(processed, _ int) {
				progress = append(progress, processed)
			},
		)
	}()

	<-started
	<-started
	select {
	case <-started:
		t.Fatal("started more channel tests than the configured concurrency")
	default:
	}
	close(release)

	summary := <-summaryResult

	assert.Equal(t, int32(2), maxActive.Load())
	assert.Equal(t, channelTestSummary{Tested: 4, Succeeded: 4}, summary)
	assert.Equal(t, []int{0, 1, 2, 3, 4}, progress)
}

func TestRunChannelTestWorkersStopsAfterCancellation(t *testing.T) {
	originalInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = originalInterval })

	ctx, cancel := context.WithCancel(context.Background())
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
		{Id: 4, Status: common.ChannelStatusEnabled},
	}
	started := make(chan struct{}, len(channels))
	progress := make([]int, 0, 1)
	summaryResult := make(chan channelTestSummary, 1)

	go func() {
		summaryResult <- runChannelTestWorkers(
			ctx,
			channels,
			2,
			func(ctx context.Context, _ *model.Channel) channelTestSummary {
				started <- struct{}{}
				<-ctx.Done()
				return channelTestSummary{Tested: 1, Succeeded: 1}
			},
			func(processed, _ int) {
				progress = append(progress, processed)
			},
		)
	}()

	<-started
	<-started
	cancel()

	summary := <-summaryResult

	select {
	case <-started:
		t.Fatal("started another channel test after cancellation")
	default:
	}
	assert.Equal(t, channelTestSummary{Tested: 2, Succeeded: 2}, summary)
	assert.Equal(t, []int{0}, progress)
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}

func setupManualChannelTestErrorTest(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// :memory: 下每个连接是独立库，单连接保证异步禁用协程与断言读到同一份数据
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.User{}, &model.Ability{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	// 禁用渠道后的通知协程会查询 root 用户；Setting 允许未配置价格的测试模型
	require.NoError(t, db.Create(&model.User{
		Username: "root", Role: common.RoleRootUser, Status: common.UserStatusEnabled,
		Setting: `{"accept_unset_model_ratio_model":true}`,
	}).Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	// 通知路径按 RedisEnabled 分流；测试进程未初始化 Redis，须显式走内存限流
	redisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	autoDisableEnabled := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	keywords := operation_setting.AutomaticDisableKeywords
	operation_setting.AutomaticDisableKeywordsFromString("insufficient balance")
	errorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = false

	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.MemoryCacheEnabled = memoryCacheEnabled
		common.RedisEnabled = redisEnabled
		common.AutomaticDisableChannelEnabled = autoDisableEnabled
		operation_setting.AutomaticDisableKeywords = keywords
		constant.ErrorLogEnabled = errorLogEnabled
	})
}

func newManualTestContext(usingKey string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyChannelKey, usingKey)
	return c
}

// TestTestChannelHandlerCoolsKeyOnUpstreamQuotaError drives the full manual
// test handler against a stub upstream returning 429 insufficient quota, and
// protects the wiring: the used key must get the limit-pattern cooldown even
// though the upstream error also sets localErr (which makes TestChannel return
// early to the client).
func TestTestChannelHandlerCoolsKeyOnUpstreamQuotaError(t *testing.T) {
	setupManualChannelTestErrorTest(t)
	// 限额模式冷却不应依赖全局自动禁用开关
	common.AutomaticDisableChannelEnabled = false

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Your token-plan quota has been exhausted.","type":"insufficient_quota","code":"insufficient_quota"}}`))
	}))
	t.Cleanup(upstream.Close)
	upstreamURL := upstream.URL

	channel := &model.Channel{
		Name:     "aliyun-quota-e2e",
		Type:     constant.ChannelTypeOpenAI,
		Key:      "upk1",
		Status:   common.ChannelStatusEnabled,
		Models:   "qwen-e2e",
		Group:    "default",
		BaseURL:  &upstreamURL,
		Priority: func() *int64 { p := int64(0); return &p }(),
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
			MultiKeyMode: constant.MultiKeyModeRandom,
			MultiKeyLimitPatterns: []model.LimitPattern{
				{Name: "aliyun", Regex: "Your token-plan quota has been exhausted", ResetCycle: "monthly:1"},
			},
		},
	}
	require.NoError(t, model.DB.Create(channel).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/"+strconv.Itoa(channel.Id), nil)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}

	TestChannel(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "token-plan quota has been exhausted")

	// 冷却同步落库，无需等待异步任务
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusTempDisabled, stored.ChannelInfo.MultiKeyStatusList[0],
		"used key should be cooled down by the limit pattern")
	assert.Greater(t, stored.ChannelInfo.MultiKeyCooldownUntil[0], common.GetTimestamp())
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status, "channel stays enabled while cooling")
}

func newInsufficientBalanceError() *relaytypes.NewAPIError {
	return relaytypes.NewOpenAIError(
		errors.New("insufficient balance"),
		relaytypes.ErrorCodeBadResponseStatusCode,
		http.StatusPaymentRequired,
	)
}

// TestProcessManualTestChannelErrorDisablesFailingKey protects the reported
// behavior: a manual "test connection" failure (e.g. insufficient balance)
// must auto-disable the used key just like the relay path, when the channel
// has auto-ban enabled and the error matches the disable rules. Upstream
// errors produce a testResult with BOTH localErr and newAPIError set, so the
// case mirrors that real shape.
func TestProcessManualTestChannelErrorDisablesFailingKey(t *testing.T) {
	setupManualChannelTestErrorTest(t)

	autoBan := 1
	channel := &model.Channel{
		Name:     "manual-test-multi-key",
		Type:     constant.ChannelTypeOpenAI,
		Key:      "k1\nk2",
		Status:   common.ChannelStatusEnabled,
		Models:   "gpt-4o-mini",
		Group:    "default",
		AutoBan:  &autoBan,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	require.NoError(t, model.DB.Create(channel).Error)

	upstreamErr := newInsufficientBalanceError()
	processManualTestChannelError(channel, testResult{
		context:     newManualTestContext("k1"),
		localErr:    upstreamErr,
		newAPIError: upstreamErr,
	})

	require.Eventually(t, func() bool {
		var stored model.Channel
		if err := model.DB.First(&stored, channel.Id).Error; err != nil {
			return false
		}
		return stored.ChannelInfo.MultiKeyStatusList[0] == common.ChannelStatusAutoDisabled
	}, 5*time.Second, 20*time.Millisecond, "used key should be auto-disabled")

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Contains(t, stored.ChannelInfo.MultiKeyDisabledReason[0], "insufficient balance")
	// 另一个 key 仍然可用，渠道不应被整体禁用
	assert.NotContains(t, stored.ChannelInfo.MultiKeyStatusList, 1)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
}

// TestProcessManualTestChannelErrorDisablesByStatusCode covers the disable
// rule driven by the upstream status code (402) rather than keywords, which
// only works when the test keeps the real upstream status code.
func TestProcessManualTestChannelErrorDisablesByStatusCode(t *testing.T) {
	setupManualChannelTestErrorTest(t)
	operation_setting.AutomaticDisableKeywordsFromString("")
	previousRanges := operation_setting.AutomaticDisableStatusCodeRanges
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 402, End: 402}}
	t.Cleanup(func() {
		operation_setting.AutomaticDisableStatusCodeRanges = previousRanges
	})

	autoBan := 1
	channel := &model.Channel{
		Name:     "manual-test-status-code",
		Type:     constant.ChannelTypeOpenAI,
		Key:      "k1\nk2",
		Status:   common.ChannelStatusEnabled,
		Models:   "gpt-4o-mini",
		Group:    "default",
		AutoBan:  &autoBan,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}
	require.NoError(t, model.DB.Create(channel).Error)

	upstreamErr := newInsufficientBalanceError()
	processManualTestChannelError(channel, testResult{
		context:     newManualTestContext("k1"),
		localErr:    upstreamErr,
		newAPIError: upstreamErr,
	})

	require.Eventually(t, func() bool {
		var stored model.Channel
		if err := model.DB.First(&stored, channel.Id).Error; err != nil {
			return false
		}
		return stored.ChannelInfo.MultiKeyStatusList[0] == common.ChannelStatusAutoDisabled
	}, 5*time.Second, 20*time.Millisecond, "used key should be auto-disabled by status code rule")
}

// TestProcessManualTestChannelErrorKeyLimitCooldownWins mirrors the relay
// semantics: when a multi-key limit pattern matches, the key gets a cooldown
// (temp disabled) instead of a hard disable, even with auto-ban enabled.
func TestProcessManualTestChannelErrorKeyLimitCooldownWins(t *testing.T) {
	setupManualChannelTestErrorTest(t)

	autoBan := 1
	channel := &model.Channel{
		Name:     "manual-test-limit-pattern",
		Type:     constant.ChannelTypeOpenAI,
		Key:      "k1\nk2",
		Status:   common.ChannelStatusEnabled,
		Models:   "gpt-4o-mini",
		Group:    "default",
		AutoBan:  &autoBan,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModeRandom,
			MultiKeyLimitPatterns: []model.LimitPattern{
				{Name: "balance", Regex: "insufficient balance", ResetCycle: "monthly:1"},
			},
		},
	}
	require.NoError(t, model.DB.Create(channel).Error)

	processManualTestChannelError(channel, testResult{
		context:     newManualTestContext("k1"),
		newAPIError: newInsufficientBalanceError(),
	})

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusTempDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Greater(t, stored.ChannelInfo.MultiKeyCooldownUntil[0], common.GetTimestamp())
	assert.Contains(t, stored.ChannelInfo.MultiKeyDisabledReason[0], "balance")
}

func TestProcessManualTestChannelErrorSkipsInapplicableCases(t *testing.T) {
	setupManualChannelTestErrorTest(t)

	autoBan := 1
	disabledChannel := &model.Channel{
		Name: "already-disabled", Type: constant.ChannelTypeOpenAI, Key: "k1",
		Status: common.ChannelStatusManuallyDisabled, Models: "gpt-4o-mini", Group: "default",
		AutoBan: &autoBan,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 1, MultiKeyMode: constant.MultiKeyModeRandom},
	}
	require.NoError(t, model.DB.Create(disabledChannel).Error)
	processManualTestChannelError(disabledChannel, testResult{
		context:     newManualTestContext("k1"),
		newAPIError: newInsufficientBalanceError(),
	})

	// AutoBan 带 gorm:"default:1"，必须显式置 0 才能构造"未开启自动禁用"的渠道
	autoBanOff := 0
	enabledNoAutoBan := &model.Channel{
		Name: "no-autoban", Type: constant.ChannelTypeOpenAI, Key: "k1",
		Status: common.ChannelStatusEnabled, Models: "gpt-4o-mini", Group: "default",
		AutoBan:      &autoBanOff,
		ChannelInfo:  model.ChannelInfo{IsMultiKey: true, MultiKeySize: 1, MultiKeyMode: constant.MultiKeyModeRandom},
	}
	require.NoError(t, model.DB.Create(enabledNoAutoBan).Error)
	processManualTestChannelError(enabledNoAutoBan, testResult{
		context:     newManualTestContext("k1"),
		newAPIError: newInsufficientBalanceError(),
	})

	enabled := &model.Channel{
		Name: "local-error", Type: constant.ChannelTypeOpenAI, Key: "k1",
		Status: common.ChannelStatusEnabled, Models: "gpt-4o-mini", Group: "default",
		AutoBan: &autoBan,
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 1, MultiKeyMode: constant.MultiKeyModeRandom},
	}
	require.NoError(t, model.DB.Create(enabled).Error)
	processManualTestChannelError(enabled, testResult{
		context:  newManualTestContext("k1"),
		localErr: errors.New("request build failed"),
	})
	processManualTestChannelError(enabled, testResult{context: newManualTestContext("k1")})

	for _, ch := range []*model.Channel{disabledChannel, enabledNoAutoBan, enabled} {
		var stored model.Channel
		require.NoError(t, model.DB.First(&stored, ch.Id).Error)
		assert.Empty(t, stored.ChannelInfo.MultiKeyStatusList, "channel %s should stay untouched", ch.Name)
		assert.Equal(t, ch.Status, stored.Status)
	}
}
